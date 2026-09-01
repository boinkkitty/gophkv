//go:build unix

package keyval

import (
	"os"
	"path"
	"syscall"
)

// createFileSync opens or creates a file and fsyncs its directory.
func createFileSync(file string) (*os.File, error) {
	fp, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	if err = syncDir(path.Base(file)); err != nil {
		_ = fp.Close()
		return nil, err
	}
	return fp, err
}

// syncDir fsyncs the directory containing a recently created file.
func syncDir(file string) error {
	flags := os.O_RDONLY | syscall.O_DIRECTORY
	dirfd, err := syscall.Open(path.Dir(file), flags, 0o644)
	if err != nil {
		return err
	}
	defer syscall.Close(dirfd)
	return syscall.Fsync(dirfd)
}
