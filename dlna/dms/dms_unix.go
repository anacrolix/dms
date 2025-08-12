//go:build linux || darwin
// +build linux darwin

package dms

import (
	"io/fs"
	"path/filepath"
	"strings"
)

func isHiddenPath(fsys fs.FS, path string) (bool, error) {
	if path == "." {
		return false, nil
	}
	base := filepath.Base(path)
	if strings.HasPrefix(base, ".") {
		return true, nil
	}

	return isHiddenPath(fsys, filepath.Dir(path))
}
