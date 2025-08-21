//go:build linux || darwin
// +build linux darwin

package dms

import "testing"

func TestIsHiddenPath(t *testing.T) {
	data := map[string]bool{
		"some/path":         false,
		"some/foo.bar":      false,
		"some/path/.hidden": true,
		"some/.hidden/path": true,
		".hidden/path":      true,
	}
	for path, expected := range data {
		if actual, err := isHiddenPath(nil, path); err != nil {
			t.Errorf("isHiddenPath(nil, %v) returned unexpected error: %s", path, err)
		} else if expected != actual {
			t.Errorf("isHiddenPath(nil, %v), expected %v, got %v", path, expected, actual)
		}
	}
}
