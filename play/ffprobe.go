package main

import (
	"log"
	"flag"
	"fmt"
	"bitbucket.org/anacrolix/dms/ffmpeg"
)

func main() {
	log.SetFlags(log.Llongfile)
	flag.Parse()
	for _, path := range flag.Args() {
		fmt.Println(ffmpeg.Probe(path))
	}
}

