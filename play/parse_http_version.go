package main

import (
	"net/http"
	"fmt"
	"strings"
)

func main() {
	fmt.Println(http.ParseHTTPVersion(strings.TrimSpace("HTTP/1.1 ")))
}
