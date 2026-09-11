package store

import (
	"os"
	"path/filepath"
)

// WriteAtomic writes via a temp file in the same directory followed by rename.
// A half-written task file would be a parse error on the next read, and the
// editor may be watching the path.
func WriteAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		f.Close()
		os.Remove(tmp) // no-op once the rename succeeded
	}()
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
