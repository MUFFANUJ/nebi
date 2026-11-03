package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nebari-dev/nebi/internal/models"
)

func TestManagedProjectRejectsExternalPixiAdd(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("requires Unix filesystem permissions and an unprivileged user")
	}
	pixiPath, err := exec.LookPath("pixi")
	if err != nil {
		t.Skip("pixi is not installed")
	}
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	project := createReadyProject(t, svc, db, "immutable", userID)
	manifest := "[workspace]\nname = \"immutable\"\nchannels = []\nplatforms = []\n"
	lock := "version: 6\nenvironments: {}\npackages: []\n"
	if _, err := svc.PushVersion(context.Background(), project.ID.String(), PushRequest{
		PixiToml: manifest,
		PixiLock: lock,
	}, userID); err != nil {
		t.Fatal(err)
	}
	dir := svc.GetProjectPath(project)
	// Existing installations already have a writable .pixi directory. It
	// must not allow Pixi to replace the protected manifest in its parent.
	if err := svc.WithProjectWriteAccess(project, func() error {
		return os.MkdirAll(filepath.Join(dir, ".pixi"), 0o755)
	}); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(pixiPath, "add", "--manifest-path", filepath.Join(dir, "pixi.toml"), "ipykernel")
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(strings.ToLower(string(out)), "permission denied") {
		t.Fatalf("external pixi add must fail on protected specs, got %v: %s", err, out)
	}
	for filename, want := range map[string]string{"pixi.toml": manifest, "pixi.lock": lock} {
		got, err := os.ReadFile(filepath.Join(dir, filename))
		if err != nil || string(got) != want {
			t.Fatalf("%s changed: %q, %v", filename, got, err)
		}
	}
	versions, err := svc.ListVersions(project.ID.String())
	if err != nil || len(versions) != 1 {
		t.Fatalf("rejected edit should leave one version, got %d, %v", len(versions), err)
	}
}

func TestProtectManagedProjects(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	for _, source := range []string{"managed", "local"} {
		project := createReadyProject(t, svc, db, source, userID)
		if err := db.Model(project).Update("source", source).Error; err != nil {
			t.Fatal(err)
		}
		dir := svc.GetProjectPath(project)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "pixi.toml"), []byte("original"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.ProtectManagedProjects(); err != nil {
		t.Fatal(err)
	}
	var projects []models.Project
	if err := db.Find(&projects).Error; err != nil {
		t.Fatal(err)
	}
	for i := range projects {
		project := &projects[i]
		dir := svc.GetProjectPath(project)
		for _, path := range []string{dir, filepath.Join(dir, "pixi.toml")} {
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			writable := info.Mode().Perm()&0o200 != 0
			if writable != (project.Source == "local") {
				t.Fatalf("unexpected permissions for %s project: %s %v", project.Source, path, info.Mode())
			}
		}
		if err := svc.SavePixiToml(project.ID.String(), "[workspace]\nname = \"authorized-draft\"\n"); err != nil {
			t.Fatalf("Nebi should still be able to save drafts: %v", err)
		}
	}
}
