package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/audit"
	nebicrypto "github.com/nebari-dev/nebi/internal/crypto"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

const minBuildEnvValueLength = 8

// BuildEnvPolicy defines which non-sensitive environment variable names users
// may configure for package-manager builds.
type BuildEnvPolicy struct {
	AllowedNames    []string
	AllowedPrefixes []string
}

var (
	buildEnvKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

	defaultBuildEnvPolicy = BuildEnvPolicy{
		AllowedNames:    []string{"GITLAB_TOKEN", "GITHUB_TOKEN", "GH_TOKEN"},
		AllowedPrefixes: []string{"NEBI_"},
	}

	reservedBuildEnvKeys = map[string]struct{}{
		"HOME": {},
		"PATH": {},
	}

	reservedBuildEnvKeyPrefixes = []string{"LD_", "DYLD_", "XDG_", "PIXI_"}
)

var workspaceArtifactFilenames = []string{"pixi.toml", "pixi.lock"}

// BuildEnvVarResult is the public shape for a configured build variable.
type BuildEnvVarResult struct {
	ID        uuid.UUID `json:"id"`
	Key       string    `json:"key"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BuildEnvVarReq holds parameters for creating or updating a build variable.
type BuildEnvVarReq struct {
	Key   string
	Value string
}

// ListBuildEnvVars returns public metadata for the current user's build variables.
func (s *WorkspaceService) ListBuildEnvVars(userID uuid.UUID) ([]BuildEnvVarResult, error) {
	var vars []models.BuildEnvVar
	if err := s.db.Where("user_id = ?", userID).Order("key ASC").Find(&vars).Error; err != nil {
		return nil, fmt.Errorf("fetch build environment variables: %w", err)
	}

	result := make([]BuildEnvVarResult, len(vars))
	for i, v := range vars {
		result[i] = buildEnvVarToResult(v)
	}
	return result, nil
}

// UpsertBuildEnvVar creates or replaces a single current-user build variable.
func (s *WorkspaceService) UpsertBuildEnvVar(userID uuid.UUID, req BuildEnvVarReq) (*BuildEnvVarResult, error) {
	key, err := normalizeBuildEnvKey(req.Key)
	if err != nil {
		return nil, err
	}
	if err := s.validateBuildEnvKeyAllowed(key); err != nil {
		return nil, err
	}
	if err := validateBuildEnvValue(req.Value); err != nil {
		return nil, err
	}

	encryptedValue, err := nebicrypto.EncryptField(req.Value, s.encKey)
	if err != nil {
		return nil, fmt.Errorf("encrypt build environment variable: %w", err)
	}

	var envVar models.BuildEnvVar
	auditAction := audit.ActionRotateBuildEnvVar
	err = s.db.Where("user_id = ? AND key = ?", userID, key).First(&envVar).Error
	switch {
	case err == nil:
		envVar.Value = encryptedValue
		if err := s.db.Save(&envVar).Error; err != nil {
			return nil, fmt.Errorf("update build environment variable: %w", err)
		}
	case errors.Is(err, gorm.ErrRecordNotFound):
		auditAction = audit.ActionCreateBuildEnvVar
		envVar = models.BuildEnvVar{
			UserID: userID,
			Key:    key,
			Value:  encryptedValue,
		}
		if err := s.db.Create(&envVar).Error; err != nil {
			if isUniqueConstraintError(err) {
				envVar, err = s.updateBuildEnvVarValue(userID, key, encryptedValue)
				if err != nil {
					return nil, fmt.Errorf("upsert build environment variable after create conflict: %w", err)
				}
				auditAction = audit.ActionRotateBuildEnvVar
			} else {
				return nil, fmt.Errorf("create build environment variable: %w", err)
			}
		}
	default:
		return nil, err
	}

	audit.LogAction(s.db, userID, auditAction, fmt.Sprintf("build_env_var:%s", envVar.ID.String()), map[string]interface{}{
		"key": key,
	})

	result := buildEnvVarToResult(envVar)
	return &result, nil
}

// DeleteBuildEnvVar removes a current-user build variable by key.
func (s *WorkspaceService) DeleteBuildEnvVar(userID uuid.UUID, key string) error {
	normalizedKey, err := normalizeBuildEnvKey(key)
	if err != nil {
		return err
	}

	var envVar models.BuildEnvVar
	if err := s.db.Where("user_id = ? AND key = ?", userID, normalizedKey).First(&envVar).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	if err := s.db.Delete(&envVar).Error; err != nil {
		return fmt.Errorf("delete build environment variable: %w", err)
	}
	audit.LogAction(s.db, userID, audit.ActionDeleteBuildEnvVar, fmt.Sprintf("build_env_var:%s", envVar.ID.String()), map[string]interface{}{
		"key": normalizedKey,
	})
	return nil
}

// BuildEnvironmentSecretsForUser returns decrypted build variables and their values for leak checks.
func (s *WorkspaceService) BuildEnvironmentSecretsForUser(userID uuid.UUID) (map[string]string, []string, error) {
	return s.buildEnvironmentForUser(userID)
}

func (s *WorkspaceService) buildEnvironmentForUser(userID uuid.UUID) (map[string]string, []string, error) {
	var vars []models.BuildEnvVar
	if err := s.db.Where("user_id = ?", userID).Order("key ASC").Find(&vars).Error; err != nil {
		return nil, nil, fmt.Errorf("fetch build environment variables: %w", err)
	}

	env := make(map[string]string, len(vars))
	for _, v := range vars {
		if err := s.validateBuildEnvKeyAllowed(v.Key); err != nil {
			return nil, nil, &ValidationError{
				Message: fmt.Sprintf("build environment variable %q is not allowed by the configured build environment policy; delete it before running builds", v.Key),
			}
		}
		value, err := nebicrypto.DecryptField(v.Value, s.encKey)
		if err != nil {
			return nil, nil, fmt.Errorf("decrypt build environment variable %q: %w", v.Key, err)
		}
		if err := validateBuildEnvValue(value); err != nil {
			return nil, nil, &ValidationError{
				Message: fmt.Sprintf("build environment variable %q has an invalid value: %s", v.Key, err.Error()),
			}
		}
		env[v.Key] = value
	}
	return env, buildEnvironmentSecretValues(env), nil
}

// buildEnvironmentSecretValues returns deterministic, non-empty values to check before persisting artifacts.
func buildEnvironmentSecretValues(env map[string]string) []string {
	seen := make(map[string]bool, len(env))
	for _, value := range env {
		if value != "" {
			seen[value] = true
		}
	}

	values := make([]string, 0, len(seen))
	for value := range seen {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

// ReadWorkspaceArtifactContents reads the Pixi files that can be persisted or published.
func ReadWorkspaceArtifactContents(wsPath string) (map[string]string, error) {
	contents := make(map[string]string, len(workspaceArtifactFilenames))
	for _, filename := range workspaceArtifactFilenames {
		content, err := os.ReadFile(filepath.Join(wsPath, filename))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", filename, err)
		}
		contents[filename] = string(content)
	}
	return contents, nil
}

// CheckBuildEnvironmentSecretLeak rejects artifacts that contain a configured build variable value.
func CheckBuildEnvironmentSecretLeak(contents map[string]string, secretValues []string) error {
	if len(contents) == 0 || len(secretValues) == 0 {
		return nil
	}

	filenames := make([]string, 0, len(contents))
	for filename := range contents {
		filenames = append(filenames, filename)
	}
	sort.Strings(filenames)

	for _, filename := range filenames {
		content := contents[filename]
		for _, secretValue := range secretValues {
			if secretValue != "" && strings.Contains(content, secretValue) {
				return &ValidationError{
					Message: fmt.Sprintf("build environment secret value leaked into %s; refusing to persist or publish artifact", filename),
				}
			}
		}
	}

	return nil
}

// EnsureNoBuildEnvironmentSecretLeak checks artifact contents against the current user's build variables.
func (s *WorkspaceService) EnsureNoBuildEnvironmentSecretLeak(userID uuid.UUID, contents map[string]string) error {
	_, secretValues, err := s.buildEnvironmentForUser(userID)
	if err != nil {
		return err
	}
	return CheckBuildEnvironmentSecretLeak(contents, secretValues)
}

func normalizeBuildEnvKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", &ValidationError{Message: "key is required"}
	}
	if !buildEnvKeyPattern.MatchString(key) {
		return "", &ValidationError{Message: "key must be a valid environment variable name"}
	}
	return key, nil
}

func (s *WorkspaceService) validateBuildEnvKeyAllowed(key string) error {
	upperKey := strings.ToUpper(key)
	if _, ok := reservedBuildEnvKeys[upperKey]; ok {
		return &ValidationError{Message: fmt.Sprintf("key %q is reserved and cannot be used for build environment variables", key)}
	}
	for _, prefix := range reservedBuildEnvKeyPrefixes {
		if strings.HasPrefix(upperKey, prefix) {
			return &ValidationError{Message: fmt.Sprintf("key %q uses reserved prefix %q", key, prefix)}
		}
	}
	if _, ok := s.buildEnvAllowedNames[key]; ok {
		return nil
	}
	for _, prefix := range s.buildEnvAllowedPrefixes {
		if strings.HasPrefix(key, prefix) {
			return nil
		}
	}
	allowed := make([]string, 0, len(s.buildEnvAllowedNames)+len(s.buildEnvAllowedPrefixes))
	for name := range s.buildEnvAllowedNames {
		allowed = append(allowed, name)
	}
	for _, prefix := range s.buildEnvAllowedPrefixes {
		allowed = append(allowed, prefix+"*")
	}
	sort.Strings(allowed)
	return &ValidationError{Message: fmt.Sprintf("key %q is not allowed; allowed names or prefixes: %s", key, strings.Join(allowed, ", "))}
}

func normalizeBuildEnvPolicy(policy BuildEnvPolicy) (map[string]struct{}, []string) {
	if len(policy.AllowedNames) == 0 && len(policy.AllowedPrefixes) == 0 {
		policy = defaultBuildEnvPolicy
	}

	names := make(map[string]struct{}, len(policy.AllowedNames))
	for _, name := range policy.AllowedNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		names[name] = struct{}{}
	}

	prefixSeen := make(map[string]struct{}, len(policy.AllowedPrefixes))
	prefixes := make([]string, 0, len(policy.AllowedPrefixes))
	for _, prefix := range policy.AllowedPrefixes {
		prefix = strings.TrimSpace(prefix)
		if prefix == "" {
			continue
		}
		if _, ok := prefixSeen[prefix]; ok {
			continue
		}
		prefixSeen[prefix] = struct{}{}
		prefixes = append(prefixes, prefix)
	}
	sort.Strings(prefixes)
	if len(names) == 0 && len(prefixes) == 0 {
		return normalizeBuildEnvPolicy(defaultBuildEnvPolicy)
	}
	return names, prefixes
}

func validateBuildEnvValue(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return &ValidationError{Message: "value is required"}
	}
	if utf8.RuneCountInString(trimmed) < minBuildEnvValueLength {
		return &ValidationError{Message: fmt.Sprintf("value must be at least %d characters", minBuildEnvValueLength)}
	}
	if strings.ContainsFunc(value, unicode.IsControl) {
		return &ValidationError{Message: "value must not contain control characters"}
	}
	return nil
}

func (s *WorkspaceService) updateBuildEnvVarValue(userID uuid.UUID, key, encryptedValue string) (models.BuildEnvVar, error) {
	var envVar models.BuildEnvVar
	if err := s.db.Where("user_id = ? AND key = ?", userID, key).First(&envVar).Error; err != nil {
		return models.BuildEnvVar{}, err
	}
	envVar.Value = encryptedValue
	if err := s.db.Save(&envVar).Error; err != nil {
		return models.BuildEnvVar{}, fmt.Errorf("update build environment variable: %w", err)
	}
	return envVar, nil
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") || strings.Contains(message, "duplicate")
}

func buildEnvVarToResult(v models.BuildEnvVar) BuildEnvVarResult {
	return BuildEnvVarResult{
		ID:        v.ID,
		Key:       v.Key,
		CreatedAt: v.CreatedAt,
		UpdatedAt: v.UpdatedAt,
	}
}
