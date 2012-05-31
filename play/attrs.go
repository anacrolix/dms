package main

import (
	"encoding/xml"
	"fmt"
)

type Meh struct {
	XMLName xml.Name
	Attrs []xml.Attr `xml:",attr"`
}

func main() {
	data, err := xml.Marshal(Meh{
		Attrs: []xml.Attr{
			xml.Attr{xml.Name{Local: "hi"}, "there"},
		},
	})
	fmt.Println(string(data), err)
}

