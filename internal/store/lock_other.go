//go:build !unix

package store

// LockFile is a no-op on platforms without flock; the tool targets unix.
func LockFile(path string) (func(), error) { return func() {}, nil }
