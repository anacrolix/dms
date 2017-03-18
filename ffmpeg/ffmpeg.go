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
		err = errors.New("missing value")
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

// Returns the last line in r. ok is false if there are no lines. err is any
// error that occurs during scanning.
func lastLine(r io.Reader) (line string, ok bool, err error) {
	s := bufio.NewScanner(r)
	s.Split(bufio.ScanLines)
	for s.Scan() {
		line = s.Text()
		ok = true
	}
	err = s.Err()
	return
}

// Returns a channel that receives the last line in r.
func lastLineCh(r io.Reader) <-chan string {
	ch := make(chan string, 1)
	go func() {
		defer close(ch)
		line, ok, err := lastLine(r)
		switch err {
		case nil:
		case io.ErrClosedPipe:
		default:
			panic(err)
		}
		if ok {
			ch <- line
		}
	}()
	return ch
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

func (me *ProbeCmd) runner(stdout, stderr io.ReadCloser) {
	defer close(me.Done)
	lastErrLineCh := lastLineCh(stderr)
	d := json.NewDecoder(bufio.NewReader(stdout))
	decodeErr := d.Decode(&me.Info)
	stdout.Close()
	waitErr := me.Cmd.Wait()
	stderr.Close()
	if waitErr == nil {
		me.Err = decodeErr
		return
	}
	lastErrLine, lastErrLineOk := <-lastErrLineCh
	if lastErrLineOk {
		me.Err = fmt.Errorf("%s: %s", waitErr, lastErrLine)
	} else {
		me.Err = waitErr
	}
	return
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
	go ret.runner(stdout, stderr)
	return
}
