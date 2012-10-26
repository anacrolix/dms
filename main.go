package main

import (
	"bitbucket.org/anacrolix/dms/dlna/dms"
	"flag"
	"fmt"
	"log"
	"os"
)

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)

	var path string
	flag.StringVar(&path, "path", func() (pwd string) {
		pwd, err := os.Getwd()
		if err != nil {
			log.Println(err)
		}
		if pwd == "" {
			pwd = "."
		}
		return
	}(), "browse root path")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "%s: %s\n", "unexpected positional arguments", flag.Args())
		flag.Usage()
		os.Exit(2)
	}

	server, err := dms.New(path)
	if server != nil {
		defer server.Close()
	}
	if err != nil {
		log.Fatalln(err)
	}
	server.Serve()
}
