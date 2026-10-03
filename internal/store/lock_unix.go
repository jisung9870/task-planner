//go:build unix

package store

import (
	"os"
	"syscall"
)

// LockFile takes an exclusive advisory lock, held until the returned func runs.
// TUI, CLI and the watcher can all write the index; serialising avoids a torn
// file that would force a full rebuild.
func LockFile(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}
