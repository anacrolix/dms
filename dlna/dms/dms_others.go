//go:build !linux && !darwin && !windows
// +build !linux,!darwin,!windows

package dms

import "io/fs"

func isHiddenPath(fsys fs.FS, path string) (bool, error) {
	return false, nil
}
