//+build !unix,!windows

package dms

import (
	"os"
)

func isHiddenPath(path string) (bool, error) {
	return false, nil
}

func isReadablePath(path string) (bool, error) {
	// Ugly but portable wait to check if we can open a file/directory
	if fh, err := os.Open(path); err == nil {
		fh.Close()
		return true, nil
	} else if !os.IsPermission(err) {
		return false, err
	}
	return false, nil
}
