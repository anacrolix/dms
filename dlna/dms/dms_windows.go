//go:build windows
// +build windows

package dms

import (
	"io/fs"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/windows"
)

const hiddenAttributes = windows.FILE_ATTRIBUTE_HIDDEN | windows.FILE_ATTRIBUTE_SYSTEM

func isHiddenPath(fsys *fs.FS, path string) (hidden bool, err error) {
	if path == "." {
		return false, nil
	}
	f, err := (*fsys).Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return false, err
	}
	// Extract the Win32FileAttributeData from Sys()
	sys, ok := fi.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return false, nil // Not a Windows file system? Default to non-hidden.
	}
	if (sys.FileAttributes & hiddenAttributes) != 0 {
		return true, nil
	}

	return isHiddenPath(fsys, filepath.ToSlash(filepath.Dir(path)))
}
