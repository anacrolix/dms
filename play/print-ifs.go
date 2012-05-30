package main

import (
	"log"
	"net"
)

func main() {
	ifs, err := net.Interfaces()
	if err != nil {
		panic(err)
	}
	for _, if_ := range ifs {
		log.Println(if_)
		addrs, err := if_.Addrs()
		if err != nil {
			panic(err)
		}
		for _, addr := range addrs {
			log.Printf("\t%s\n", addr)
		}
		mcastAddrs, err := if_.MulticastAddrs()
		if err != nil {
			panic(err)
		}
		for _, addr := range mcastAddrs {
			log.Printf("\t%s\n", addr)
		}
	}
}
