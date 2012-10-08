package ffmpeg

import (
	"bitbucket.org/anacrolix/dms/cache"
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
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
			return nil, fmt.Errorf("bad line: %s", line)
		}
		opt := ss[0]
		val := ss[1]
		if _, ok := ret[opt]; ok {
			return nil, errors.New(fmt.Sprint("duplicate option:", opt))
		}
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

func init() {
	var err error
	if ffprobePath, err = exec.LookPath("ffprobe"); err != nil {
		log.Println(err)
	}
}

func probeUncached(path string) (info *Info, err error) {
	if ffprobePath == "" {
		return nil, nil
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
			return nil, errors.New(fmt.Sprint("unknown section:", line))
		}
	}
	return info, nil
}

type probeStamp time.Time

var probeCache *cache.Cache = cache.New()

func Probe(path string) (info *Info, err error) {
	fi, err := os.Stat(path)
	if err != nil {
		return
	}
	stamp := fi.ModTime()
	data, err := probeCache.Get(path, stamp, func() (cache.Data, cache.Stamp, error) {
		info, err := probeUncached(path)
		return info, stamp, err
	})
	info = data.(*Info)
	return
}
