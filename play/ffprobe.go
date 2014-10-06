// +build ignore

package main

import (
	"bitbucket.org/anacrolix/dms/ffmpeg"
	"flag"
	"fmt"
	"log"
)

func main() {
	log.SetFlags(log.Llongfile)
	flag.Parse()
	for _, path := range flag.Args() {
		fmt.Println(ffmpeg.Probe(path))
	}
}
