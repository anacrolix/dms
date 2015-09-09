// +build ignore

package main

import (
	"flag"
	"log"

	"github.com/anacrolix/dms/ffmpeg"
)

func main() {
	log.SetFlags(log.Llongfile)
	flag.Parse()
	for _, path := range flag.Args() {
		i, err := ffmpeg.Probe(path)
		log.Printf("%#v %#v", i, err)
	}
}
