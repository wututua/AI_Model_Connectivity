//go:build !linux

package update

import (
	"errors"
	"os"
)

func trustedPath(string) bool { return false }
func openRegular(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("update file is not regular")
	}
	return os.Open(path)
}
func syncDirectory(string) error    { return nil }
func workerLock() (*os.File, error) { return nil, ErrUnsupported }
