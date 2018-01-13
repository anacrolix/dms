//+build windows

package dms

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

const hiddenAttributes = windows.FILE_ATTRIBUTE_HIDDEN | windows.FILE_ATTRIBUTE_SYSTEM

func isHiddenPath(path string) (hidden bool, err error) {
	winPath, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	attrs, err := windows.GetFileAttributes(winPath)
	if err == nil {
		if attrs&hiddenAttributes != 0 {
			hidden = true
		} else {
			parent := filepath.Dir(path)
			if parent != path {
				hidden, err = isHiddenPath(parent)
			}
		}
	}
	return
}

func isReadablePath(path string) (bool, error) {
	return tryToOpenPath(path)
}
