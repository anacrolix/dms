//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
)

func main() {
	url_, err := url.Parse("[192:168:26:2::3]:1900")
	if err != nil {
		slog.Error("error parsing URL", "error", err)
		os.Exit(1)
	}
	fmt.Printf("%#v\n", url_)
}
