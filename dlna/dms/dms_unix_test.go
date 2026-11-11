//go:build linux || darwin
// +build linux darwin

package dms

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

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

func TestReadDirFollowsSymlinkToRegularFile(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "target.mp3")
	if err := os.WriteFile(targetPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(dir, "link.mp3")
	if err := os.Symlink(targetPath, linkPath); err != nil {
		t.Fatal(err)
	}

	files, err := (&object{Path: "."}).readDir(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	var linkInfo fs.FileInfo
	for _, fi := range files {
		if fi.Name() == "link.mp3" {
			linkInfo = fi
			break
		}
	}
	if linkInfo == nil {
		t.Fatal("missing symlink entry")
	}
	if !linkInfo.Mode().IsRegular() {
		t.Fatalf("expected symlink to resolve as regular file, mode=%v", linkInfo.Mode())
	}
}
