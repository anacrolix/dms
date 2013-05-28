package main

import (
	"bitbucket.org/anacrolix/dms/dlna/dms"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
)

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)

	path := flag.String("path", func() (pwd string) {
		pwd, err := os.Getwd()
		if err != nil {
			log.Print(err)
		}
		if pwd == "" {
			pwd = "."
		}
		return
	}(), "browse root path")

	ifName := flag.String("ifname", "", "specific SSDP network interface")
	httpAddr := flag.String("http", ":1338", "http server port")
	friendlyName := flag.String("friendlyName", "", "server friendly name")

	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "%s: %s\n", "unexpected positional arguments", flag.Args())
		flag.Usage()
		os.Exit(2)
	}

	(&dms.Server{
		Interfaces: func(ifName string) (ifs []net.Interface) {
			var err error
			if ifName == "" {
				ifs, err = net.Interfaces()
			} else {
				var if_ *net.Interface
				if_, err = net.InterfaceByName(ifName)
				if if_ != nil {
					ifs = append(ifs, *if_)
				}
			}
			if err != nil {
				log.Fatal(err)
			}
			return
		}(*ifName),
		HTTPConn: func() net.Listener {
			conn, err := net.Listen("tcp", *httpAddr)
			if err != nil {
				log.Fatal(err)
			}
			return conn
		}(),
		FriendlyName:   *friendlyName,
		RootObjectPath: *path,
	}).Serve()
}
