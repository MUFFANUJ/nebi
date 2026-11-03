package worker

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nebari-dev/nebi/internal/limits"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/queue"
	"gorm.io/gorm"
)

func TestInstallPackagesPreservesSavedRevision(t *testing.T) {
	for _, local := range []bool{true, false} {
		name := "team"
		if local {
			name = "local"
		}
		t.Run(name, func(t *testing.T) {
			db, svc, jobSvc, exec := setupWorkerTestMode(t, local)
			project, createJob := newTestProject(t, db, exec, "versioned-repair", models.JobTypeCreate, nil)
			exec.createManifest = "[workspace]\nname = \"versioned-repair\"\n"
			exec.createLock = "version: 6\n"
			w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), limits.Defaults(), 1)
			if err := w.executeJob(context.Background(), createJob, &bytes.Buffer{}); err != nil {
				t.Fatal(err)
			}
			original, err := svc.GetVersion(project.ID.String(), "1")
			if err != nil {
				t.Fatal(err)
			}
			exec.installHook = func(context.Context) error {
				return os.WriteFile(filepath.Join(exec.GetProjectPath(project), "pixi.toml"),
					[]byte(exec.createManifest+"[dependencies]\nipykernel = \"*\"\n"), 0o644)
			}
			exec.installLock = "version: 6\n# ipykernel\n"
			job, err := svc.InstallPackages(context.Background(), project.ID.String(), []string{"ipykernel"}, project.OwnerID)
			if err != nil {
				t.Fatal(err)
			}
			if err := w.executeJob(context.Background(), job, &bytes.Buffer{}); err != nil {
				t.Fatal(err)
			}
			previous, err := svc.GetVersion(project.ID.String(), "1")
			if err != nil {
				t.Fatal(err)
			}
			if previous.ID != original.ID || previous.ManifestContent != original.ManifestContent || previous.LockFileContent != original.LockFileContent {
				t.Fatal("install changed the saved revision")
			}
			next, err := svc.GetVersion(project.ID.String(), "2")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(next.ManifestContent, "ipykernel") || !strings.Contains(next.LockFileContent, "ipykernel") || next.JobID == nil || *next.JobID != job.ID {
				t.Fatalf("repair must be recorded in a new revision: %+v", next)
			}
			for _, path := range []string{exec.GetProjectPath(project), filepath.Join(exec.GetProjectPath(project), "pixi.toml"), filepath.Join(exec.GetProjectPath(project), "pixi.lock")} {
				info, err := os.Stat(path)
				if err != nil || info.Mode().Perm()&0o222 != 0 {
					t.Fatalf("spec must be protected after install: %s, %v", path, err)
				}
			}
		})
	}
}

func TestInstallPackagesFailsWhenSnapshotCannotBeSaved(t *testing.T) {
	db, svc, jobSvc, exec := setupWorkerTest(t)
	project, job := newTestProject(t, db, exec, "snapshot-failure", models.JobTypeInstall,
		map[string]interface{}{"packages": []string{"ipykernel"}})
	wantErr := errors.New("snapshot database unavailable")
	if err := db.Callback().Create().Before("gorm:create").Register("fail_snapshot", func(tx *gorm.DB) {
		if tx.Statement.Table == "project_versions" {
			tx.AddError(wantErr)
		}
	}); err != nil {
		t.Fatal(err)
	}
	w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), limits.Defaults(), 1)
	w.processJob(context.Background(), job)
	var stored models.Job
	if err := db.First(&stored, "id = ?", job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != models.JobStatusFailed || !strings.Contains(stored.Error, wantErr.Error()) {
		t.Fatalf("unrecorded mutation must not report success: %s, %s", stored.Status, stored.Error)
	}
	info, err := os.Stat(filepath.Join(exec.GetProjectPath(project), "pixi.toml"))
	if err != nil || info.Mode().Perm()&0o222 != 0 {
		t.Fatalf("spec must be protected after failure: %v", err)
	}
}
