package dms

import (
	"bytes"
	"net/http"
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

func TestRequest(t *testing.T) {
	resp, err := http.NewRequest("NOTIFY", "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	buf := bytes.NewBuffer(nil)
	resp.Write(buf)
	t.Logf("%q", buf.String())
}

func TestResponse(t *testing.T) {
	var resp http.Response
	resp.StatusCode = http.StatusOK
	resp.Header = make(http.Header)
	resp.Header["SID"] = []string{"uuid:1337"}
	var buf bytes.Buffer
	resp.Write(&buf)
	t.Logf("%q", buf.String())
}
