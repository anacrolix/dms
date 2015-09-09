package ffmpeg

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"
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

// Sends the last line from r to ch, or returns the error scanning r.
func lastLine(r io.Reader, ch chan<- string) (err error) {
	defer close(ch)
	scanner := bufio.NewScanner(r)
	scanner.Split(bufio.ScanLines)
	var line string
	for scanner.Scan() {
		line = scanner.Text()
	}
	err = scanner.Err()
	if err != nil {
		return
	}
	ch <- line
	return
}

// Runs ffprobe or avprobe or similar on the given file path.
func Probe(path string) (info *Info, err error) {
	pc, err := StartProbe(path)
	if err != nil {
		return
	}
	<-pc.Done
	info, err = pc.Info, pc.Err
	return
}

type ProbeCmd struct {
	Cmd  *exec.Cmd
	Done chan struct{}
	mu   sync.Mutex
	Info *Info
	Err  error
}

func StartProbe(path string) (ret *ProbeCmd, err error) {
	if ffprobePath == "" {
		err = FfprobeUnavailableError
		return
	}
	cmd := exec.Command(ffprobePath,
		"-loglevel", "error",
		"-show_format",
		"-show_streams",
		outputFormatFlag, "json",
		path)
	setHideWindow(cmd)
	var stdout, stderr *io.PipeReader
	stdout, cmd.Stdout = io.Pipe()
	stderr, cmd.Stderr = io.Pipe()
	err = cmd.Start()
	if err != nil {
		return
	}
	ret = &ProbeCmd{
		Cmd:  cmd,
		Done: make(chan struct{}),
	}
	lastLineCh := make(chan string, 1)
	ret.mu.Lock()
	go func() {
		defer close(ret.Done)
		err := cmd.Wait()
		stdout.Close()
		stderr.Close()
		ret.mu.Lock()
		defer ret.mu.Unlock()
		if err == nil {
			return
		}
		lastLine, ok := <-lastLineCh
		if ok {
			err = fmt.Errorf("%s: %s", err, lastLine)
		}
		ret.Err = err
	}()
	go lastLine(stderr, lastLineCh)
	go func() {
		decoder := json.NewDecoder(bufio.NewReader(stdout))
		ret.Err = decoder.Decode(&ret.Info)
		ret.mu.Unlock()
		stdout.Close()
	}()
	return
}
