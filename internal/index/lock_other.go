//go:build !unix

package index

// lockFile is a no-op on platforms without flock; the tool targets unix.
func lockFile(path string) (func(), error) { return func() {}, nil }
