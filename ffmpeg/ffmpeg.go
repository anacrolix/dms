package ffmpeg

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
)

type Info struct {
	Format  map[string]string
	Streams []map[string]string
}

func readSection(r *bufio.Reader, end string) (map[string]string, error) {
	ret := make(map[string]string)
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
	cmd := exec.Command(ffprobePath, "-show_format", "-show_streams", path)
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
	for {
		line, err := readLine(r)
		if err == io.EOF {
			err = nil
			break
		}
		if err != nil {
			return nil, err
		}
		switch line {
		case "[FORMAT]":
			info.Format, err = readSection(r, "[/FORMAT]")
			if err != nil {
				return nil, err
			}
		case "[STREAM]":
			m, err := readSection(r, "[/STREAM]")
			if err != nil {
				return nil, err
			}
			var i int
			if _, err := fmt.Sscan(m["index"], &i); err != nil {
				return nil, err
			}
			if i != len(info.Streams) {
				return nil, errors.New("streams unordered")
			}
			info.Streams = append(info.Streams, m)
		default:
			return nil, fmt.Errorf("unknown section: %v", line)
		}
	}
	return info, nil
}
