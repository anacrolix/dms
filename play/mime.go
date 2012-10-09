package main

import (
	"../dlna/dms"
	"flag"
	"fmt"
)

func main() {
	flag.Parse()
	for _, arg := range flag.Args() {
		fmt.Println(dms.MimeTypeByPath(arg))
	}
}
