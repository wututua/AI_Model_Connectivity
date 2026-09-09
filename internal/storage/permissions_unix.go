//go:build !windows

package storage

import "os"

func restrictFileAccess(path string) error {
	return os.Chmod(path, 0600)
}
