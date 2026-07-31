//go:build !linux && !darwin && !windows && !openbsd
// +build !linux,!darwin,!windows,!openbsd

package dms

import "io/fs"

func isHiddenPath(fsys fs.FS, path string) (bool, error) {
	return false, nil
}
