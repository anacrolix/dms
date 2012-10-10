// +build !windows

package dms

func suppressFFmpegProbeDataErrors(err error) error {
	return err
}