package misc

import (
	"bitbucket.org/anacrolix/dms/ffmpeg"
	"io"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strconv"
)

func streamArgs(s map[string]string) (ret []string) {
	defer func() {
		if len(ret) != 0 {
			ret = append(ret, []string{
				"-map", "0:" + s["index"],
			}...)
		}
	}()
	switch s["codec_type"] {
	case "video":
		/*
			if s["codec_name"] == "h264" {
				if i, _ := strconv.ParseInt(s["is_avc"], 0, 0); i != 0 {
					return []string{"-vcodec", "copy", "-sameq", "-vbsf", "h264_mp4toannexb"}
				}
			}
		*/
		return []string{"-target", "pal-dvd"}
	case "audio":
		if s["codec_name"] == "dca" {
			return []string{"-acodec", "ac3", "-ab", "224k", "-ac", "2"}
		} else {
			return []string{"-acodec", "copy"}
		}
	case "subtitle":
		return []string{"-scodec", "copy"}
	}
	return
}

func Transcode(path, ss, t string) (r io.ReadCloser, err error) {
	args := []string{
		"ffmpeg",
		"-threads", strconv.FormatInt(int64(runtime.NumCPU()), 10),
		"-async", "1",
	}
	if ss != "" {
		args = append(args, []string{
			"-ss", ss,
		}...)
	}
	if t != "" {
		args = append(args, []string{
			"-t", t,
		}...)
	}
	args = append(args, []string{
		"-i", path,
	}...)
	info, err := ffmpeg.Probe(path)
	if err != nil {
		return
	}
	for _, s := range info.Streams {
		args = append(args, streamArgs(s)...)
	}
	args = append(args, []string{"-f", "mpegts", "pipe:"}...)
	log.Println("transcode command:", args)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stderr = os.Stderr
	r, err = cmd.StdoutPipe()
	if err != nil {
		return
	}
	err = cmd.Start()
	if err != nil {
		return
	}
	go func() {
		if err := cmd.Wait(); err != nil {
			log.Println(err)
		}
	}()
	return
}
