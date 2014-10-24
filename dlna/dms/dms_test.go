package dms

import (
	"path/filepath"
	"runtime"
	"testing"
)

type safeFilePathTestCase struct {
	root, given, expected string
}

func TestSafeFilePath(t *testing.T) {
	cases := []safeFilePathTestCase{
		{"/", "..", "/"},
		{"/hello", "..//", "/hello"},
		{"", "/precious", "precious"},
		{".", "///precious", "precious"},
	}
	if runtime.GOOS == "windows" {
		cases = append(cases, []safeFilePathTestCase{
			{"c:", "/", "c:/"},
			{"c:", "/test", "c:/test"},
			{"c:/", "/", "c:/"},
			{"c:/", "/test", "c:/test"},
		}...)
	}
	t.Logf("running %d test cases", len(cases))
	for _, _case := range cases {
		e := filepath.FromSlash(_case.expected)
		a := safeFilePath(_case.root, _case.given)
		if a != e {
			t.Fatalf("expected %q from %q and %q but got %q", e, _case.root, _case.given, a)
		}
	}
}
