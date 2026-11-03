// Package projectfs protects Nebi-managed project specifications between writes.
package projectfs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var writeLocks sync.Map

// WithWriteAccess serializes writes to a managed project and makes its specs
// read-only again on every exit, including failures. The directory must also be
// protected: Pixi saves manifests by replacing files with an atomic rename.
// This prevents accidental edits by external tools, not deliberate chmod by
// the owner. User-owned local projects must not use this function.
func WithWriteAccess(path string, write func() error) (err error) {
	key, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	value, _ := writeLocks.LoadOrStore(key, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	defer func() {
		if protectErr := setWritable(path, false); protectErr != nil {
			err = errors.Join(err, protectErr)
		}
	}()
	if err := setWritable(path, true); err != nil {
		return err
	}
	return write()
}

func setWritable(path string, writable bool) error {
	for _, target := range []string{path, filepath.Join(path, "pixi.toml"), filepath.Join(path, "pixi.lock")} {
		info, err := os.Lstat(target)
		if os.IsNotExist(err) {
			continue // Creation and deletion jobs may not have a directory yet.
		}
		if err != nil {
			return fmt.Errorf("inspect project spec %s: %w", target, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || (target == path && !info.IsDir()) || (target != path && !info.Mode().IsRegular()) {
			return fmt.Errorf("project spec %s must not be a symlink or special file", target)
		}
		mode := info.Mode().Perm() &^ 0o222
		if writable {
			mode |= 0o200
		}
		if err := os.Chmod(target, mode); err != nil {
			return fmt.Errorf("set project spec permissions %s: %w", target, err)
		}
	}
	return nil
}
