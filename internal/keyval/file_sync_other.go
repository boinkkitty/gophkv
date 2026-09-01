//go:build !unix

package keyval

import "os"

// createFileSync opens or creates a file on platforms without directory fsync.
func createFileSync(file string) (*os.File, error) {
	return os.OpenFile(file, os.O_RDWR|os.O_CREATE, 0o644)
}
