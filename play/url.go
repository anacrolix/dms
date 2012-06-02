package main

import (
	"net/url"
	"fmt"
	"log"
)

func main() {
	url_, err := url.Parse("[192:168:26:2::3]:1900")
	if err != nil {
		log.Fatalln(err)
	}
	fmt.Printf("%#v\n", url_)
}

