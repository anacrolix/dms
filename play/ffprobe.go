// +build ignore

package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/anacrolix/dms/ffmpeg"
)

func main() {
	log.SetFlags(log.Llongfile)
	flag.Parse()
	for _, path := range flag.Args() {
		fmt.Println(ffmpeg.Probe(path))
	}
}
