package dms

import (
	"testing"
)

func TestSafeFilePath(t *testing.T) {
	assertEqual := func(e, a string) {
		if e != a {
			t.Fatalf("expected %q but got %q", e, a)
		}
	}
	for _, _case := range []struct {
		root, given, expected string
	}{
		{"/hello", "..//", "/hello"},
		{"", "/precious", "precious"},
		{".", "///precious", "precious"},
	} {
		assertEqual(_case.expected, safeFilePath(_case.root, _case.given))
	}
}
