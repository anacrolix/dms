//+build windows

package dms

import (
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

const hiddenAttributes = windows.FILE_ATTRIBUTE_HIDDEN | windows.FILE_ATTRIBUTE_SYSTEM

func isHiddenPath(path string) (hidden bool, err error) {
	attrs, err := windows.GetFileAttributes(toWindowsPath(path))
	if err == nil {
		hidden = attrs&hiddenAttributes != 0
	}
	return
}

func toWindowsPath(path string) *uint16 {
	utf16Path := utf16.Encode([]rune(path))
	return &(utf16Path[0])
}

func isReadablePath(path string) (bool, error) {
	return tryToOpenPath(path)
}
