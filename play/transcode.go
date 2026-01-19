//go:build ignore
// +build ignore

package main

import (
	"bufio"
	"flag"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/anacrolix/dms/misc"
)

func main() {
	ss := flag.String("ss", "", "")
	t := flag.String("t", "", "")
	flag.Parse()
	if flag.NArg() != 1 {
		slog.Error("wrong argument count")
		os.Exit(1)
	}
	r, err := misc.Transcode(flag.Arg(0), *ss, *t)
	if err != nil {
		slog.Error("transcode error", "error", err)
		os.Exit(1)
	}
	go func() {
		buf := bufio.NewWriterSize(os.Stdout, 1234)
		n, err := io.Copy(buf, r)
		slog.Info("copied bytes", "count", n)
		if err != nil {
			slog.Info("copy error", "error", err)
		}
	}()
	time.Sleep(time.Second)
	go r.Close()
	time.Sleep(time.Second)
}
