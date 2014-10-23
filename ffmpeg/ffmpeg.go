package ffmpeg

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"time"
)

type Info struct {
	Format  map[string]interface{}
	Streams []map[string]interface{}
}

// returns res attributes for the raw stream
func (info *Info) Bitrate() (bitrate uint, err error) {
	bit_rate, exist := info.Format["bit_rate"]
	if !exist {
		err = errors.New("no bit_rate key in format")
		return
	}
	_, err = fmt.Sscan(bit_rate.(string), &bitrate)
	return
}

func (info *Info) Duration() (duration time.Duration, err error) {
	di := info.Format["duration"]
	if di == nil {
		err = errors.New("no format duration")
		return
	}
	ds := di.(string)
	if ds == "N/A" {
		err = errors.New("N/A")
		return
	}
	var f float64
	_, err = fmt.Sscan(ds, &f)
	if err != nil {
		return
	}
	duration = time.Duration(f * float64(time.Second))
	return
}

var (
	ffprobePath      string
	outputFormatFlag = "-of"
)

func isExecErrNotFound(err error) bool {
	if err == exec.ErrNotFound {
		return true
	}
	execErr, ok := err.(*exec.Error)
	if !ok {
		return false
	}
	return execErr.Err == exec.ErrNotFound
}

func init() {
	var err error
	ffprobePath, err = exec.LookPath("ffprobe")
	if err == nil {
		outputFormatFlag = "-print_format"
		return
	}
	if !isExecErrNotFound(err) {
		log.Print(err)
	}
	ffprobePath, err = exec.LookPath("avprobe")
	if err == nil {
		return
	}
	if isExecErrNotFound(err) {
		log.Print("ffprobe and avprobe not found in $PATH")
		return
	}
	log.Print(err)
}

var FfprobeUnavailableError = errors.New("ffprobe not available")

func Probe(path string) (info *Info, err error) {
	if ffprobePath == "" {
		err = FfprobeUnavailableError
		return
	}
	cmd := exec.Command(ffprobePath, "-show_format", "-show_streams", outputFormatFlag, "json", path)
	setHideWindow(cmd)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	r := bufio.NewReader(out)
	info = &Info{}
	defer out.Close()
	defer func() {
		waitErr := cmd.Wait()
		if waitErr != nil {
			err = waitErr
		}
		if err != nil {
			info = nil
		}
	}()
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(info); err != nil {
		return nil, err
	}
	return info, nil
}
