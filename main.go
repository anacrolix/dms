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
	flag.StringVar(&path, "path", getWD(), "content path")

	var netInterface string
	flag.StringVar(&netInterface, "netInterface", "all", "network interface")

	var httpPort int
	flag.IntVar(&httpPort, "httpPort", 1338, "http server port")

	var friendlyName string
	flag.StringVar(&friendlyName, "friendlyName", "", "DLNA server friendly name")

	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "%s: %s\n", "unexpected positional arguments", flag.Args())
		flag.Usage()
		os.Exit(2)
	}

	server, err := dms.New(path, httpPort, friendlyName)
	if server != nil {
		defer server.Close()
	}
	if err != nil {
		log.Fatalln(err)
	}
	server.Serve(netInterface)
}

func getWD() (pwd string) {
	pwd, err := os.Getwd()
	if err != nil {
		log.Println(err)
	}
	if pwd == "" {
		pwd = "."
	}
	return
}
