package ffmpeg

import (
	"bufio"
	"encoding/json"
	"errors"
	"log"
	"os/exec"
	"strings"
)

type Info struct {
	Format  map[string]interface{}
	Streams []map[string]interface{}
}

func readSection(r *bufio.Reader, end string) (map[string]interface{}, error) {
	ret := make(map[string]interface{})
	for {
		line, err := readLine(r)
		if err != nil {
			return nil, err
		}
		if line == end {
			break
		}
		ss := strings.SplitN(line, "=", 2)
		if len(ss) != 2 {
			continue
		}
		opt := ss[0]
		val := ss[1]
		ret[opt] = val
	}
	return ret, nil
}

func readLine(r *bufio.Reader) (line string, err error) {
	for {
		var (
			buf []byte
			isP bool
		)
		buf, isP, err = r.ReadLine()
		if err != nil {
			return
		}
		line += string(buf)
		if !isP {
			break
		}
	}
	return
}

var ffprobePath string

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
	cmd := exec.Command(ffprobePath, "-show_format", "-show_streams", "-of", "json", path)
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
