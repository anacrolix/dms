package dms

import (
	"bitbucket.org/anacrolix/dms/ffmpeg"
	"strings"
	"testing"
)

func TestEscapeObjectID(t *testing.T) {
	o := object{
		Path: "/some/file",
	}
	id := o.ID()
	if strings.ContainsAny(id, "/") {
		t.Skip("may not work with some players: object IDs contain '/'")
	}
}

func TestRootObjectID(t *testing.T) {
	if (object{Path: "/"}).ID() != "0" {
		t.FailNow()
	}
}

func TestRootParentObjectID(t *testing.T) {
	if (object{Path: "/"}).ParentID() != "-1" {
		t.FailNow()
	}
}

func TestItemResExtra_BitRateMissing(t *testing.T) {
	info := &ffmpeg.Info{
		Format: map[string]interface{}{"duration": "N/A"},
	}
	excepted := uint(0)
	if br, _ := itemResExtra(info); br != excepted {
		t.Errorf("bitrate must be '%d', but we have '%d'", excepted, br)
	}
}

func TestItemResExtra_BitRateInStringFormat(t *testing.T) {
	info := &ffmpeg.Info{
		Format: map[string]interface{}{"bit_rate": "128000.000000", "duration": "N/A"},
	}
	excepted := uint(128000)
	if br, _ := itemResExtra(info); br != excepted {
		t.Errorf("bitrate must be '%d', but we have '%d'", excepted, br)
	}
}

func TestItemResExtra_DurationNA(t *testing.T) {
	info := &ffmpeg.Info{
		Format: map[string]interface{}{"duration": "N/A"},
	}
	excepted := ""
	if _, d := itemResExtra(info); d != excepted {
		t.Errorf("bitrate must be '%s', but we have '%s'", excepted, d)
	}
}

func TestItemResExtra_DurationMissing(t *testing.T) {
	info := &ffmpeg.Info{
		Format: map[string]interface{}{},
	}
	excepted := ""
	if _, d := itemResExtra(info); d != excepted {
		t.Errorf("bitrate must be '%s', but we have '%s'", excepted, d)
	}
}

func TestItemResExtra_DurationExist(t *testing.T) {
	info := &ffmpeg.Info{
		Format: map[string]interface{}{"duration": "1377.628452"},
	}
	excepted := "0:22:57.628452"
	if _, d := itemResExtra(info); d != excepted {
		t.Errorf("bitrate must be '%s', but we have '%s'", excepted, d)
	}
}
