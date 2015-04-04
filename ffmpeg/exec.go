// +build !windows

package ffmpeg

import (
	"os/exec"
)

func setHideWindow(cmd *exec.Cmd) {}
