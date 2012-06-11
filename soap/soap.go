package soap

import (
	"encoding/xml"
)

const (
	EncodingStyle = "http://schemas.xmlsoap.org/soap/encoding/"
)

type Arg struct {
	XMLName xml.Name
	Value   string `xml:",chardata"`
}

type Action struct {
	XMLName xml.Name
	Args    []Arg
}

type Body struct {
	Action []byte `xml:",innerxml"`
}

type Envelope struct {
	XMLName       xml.Name `xml:"http://schemas.xmlsoap.org/soap/envelope/ Envelope"`
	EncodingStyle string   `xml:"encodingStyle,attr"`
	Body          Body     `xml:"http://schemas.xmlsoap.org/soap/envelope/ Body"`
}

func NewEnvelope(action []byte) Envelope {
	return Envelope{
		EncodingStyle: EncodingStyle,
		Body:          Body{action},
	}
}
