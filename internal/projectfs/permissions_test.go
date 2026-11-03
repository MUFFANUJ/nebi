package projectfs

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWithWriteAccess(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("requires Unix filesystem permissions and an unprivileged user")
	}
	dir := t.TempDir()
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	manifest := filepath.Join(dir, "pixi.toml")
	lock := filepath.Join(dir, "pixi.lock")
	err := WithWriteAccess(dir, func() error {
		if err := os.WriteFile(manifest, []byte("original manifest"), 0o644); err != nil {
			return err
		}
		return os.WriteFile(lock, []byte("original lock"), 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{manifest, lock} {
		if err := os.WriteFile(path, []byte("untracked edit"), 0o644); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("direct edit of %s: expected permission denied, got %v", path, err)
		}
		replacement := filepath.Join(t.TempDir(), "replacement")
		if err := os.WriteFile(replacement, []byte("atomic replacement"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(replacement, path); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("atomic replacement of %s: expected permission denied, got %v", path, err)
		}
	}

	wantErr := errors.New("operation failed")
	err = WithWriteAccess(dir, func() error {
		if err := os.WriteFile(manifest, []byte("authorized edit"), 0o644); err != nil {
			t.Fatal(err)
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected original failure, got %v", err)
	}
	if err := os.WriteFile(manifest, []byte("untracked edit after failure"), 0o644); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("spec must be protected after failure, got %v", err)
	}
	if err := WithWriteAccess(dir, func() error { return os.RemoveAll(dir) }); err != nil {
		t.Fatalf("authorized deletion: %v", err)
	}
}

func TestWithWriteAccessRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	target := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(target, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "pixi.toml")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	if err := WithWriteAccess(dir, func() error {
		t.Fatal("must not write through symlink")
		return nil
	}); err == nil {
		t.Fatal("expected symlink rejection")
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("changed permissions outside project: %v", info.Mode())
	}
}
