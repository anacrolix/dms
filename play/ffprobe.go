//go:build ignore
// +build ignore

package main

import (
	"flag"
	"log/slog"

	"github.com/anacrolix/ffprobe"
)

func main() {
	flag.Parse()
	for _, path := range flag.Args() {
		i, err := ffprobe.Probe(path)
		slog.Info("ffprobe result", "info", i, "error", err)
	}
}
