package ffmpeg

import (
	"time"

	"testing"
)

func TestInfoHelperMethods(t *testing.T) {
	for _, tc := range []struct {
		Info          // Input data
		time.Duration // Expected duration
		uint          // Expected bitrate
	}{
		{Info{
			Format: map[string]interface{}{"duration": "N/A"},
		}, 0, 0},
		{Info{Format: map[string]interface{}{"bit_rate": "128000.000000", "duration": "N/A"}}, 0, 128000},
		{Info{}, 0, 0},
		{Info{Format: map[string]interface{}{"duration": "1377.628452"}}, 1377628452000, 0},
	} {
		br, _ := tc.Bitrate()
		if br != tc.uint {
			t.Fatalf("bitrate is %d, but expected %d", br, tc.uint)
		}
		d, _ := tc.Info.Duration()
		if d != tc.Duration {
			t.Fatalf("duration is %s but expected %s", d, tc.Duration)
		}
	}
}
