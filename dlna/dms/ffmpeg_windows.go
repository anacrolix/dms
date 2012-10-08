package dms

import (
	"os/exec"
	"syscall"
)

func suppressFFmpegProbeDataErrors(err error) error {
	if err == nil {
		return err
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		return err
	}
	waitStat, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok {
		return err
	}
	if waitStat.ExitStatus() == -1094995529 {
		return nil
	}
	return err
}
