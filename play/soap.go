package main

import (
	"encoding/xml"
	"fmt"
	"os"
	"io/ioutil"
)

type Arg struct {
	XMLName xml.Name
	Value string `xml:",chardata"`
}

type Action struct {
	XMLName xml.Name
	Args []Arg `xml:",any"`
	//Noob []Arg
}

type Body struct {
	Action Action `xml:",any"`
}

type envelope struct {
	XMLName xml.Name `xml:"http://schemas.xmlsoap.org/soap/envelope/ Envelope"`
	EncodingStyle string `xml:"encodingStyle,attr"`
	Body Body `xml:"http://schemas.xmlsoap.org/soap/envelope/ Body"`
}

func main() {
	raw, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		panic(err)
	}
	var env envelope
	if err := xml.Unmarshal(raw, &env); err != nil {
		panic(err)
	}
	fmt.Println(env)
	raw, err = xml.MarshalIndent(
		envelope{
			EncodingStyle: "http://schemas.xmlsoap.org/soap/envelope/",
			Body: Body{
				Action{
					xml.Name{
						Space: env.Body.Action.XMLName.Space,
						Local: env.Body.Action.XMLName.Local + "Response",
					},
					[]Arg{
						Arg{
							xml.Name{
								Local: "SortCaps",
							},
							"dc:title",
						},
						Arg{
							xml.Name{Local: "lol"},
							"meh",
						},
					},
				},
			},
		}, "", "  ")
	if err != nil {
		panic(err)
	}
	fmt.Println(string(raw))
}

