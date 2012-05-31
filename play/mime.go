package main

import (
	"fmt"
	"mime"
	"strings"
)

func main() {
	fmt.Println(strings.SplitN(mime.TypeByExtension(".avi"), "/", 2)[0])
}
