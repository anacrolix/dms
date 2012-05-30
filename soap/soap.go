package soap

import (
	"encoding/xml"
	"fmt"
)

const (
	EncodingStyle = "http://schemas.xmlsoap.org/soap/encoding/"
)

type Arg struct {
	XMLName xml.Name
	Value string `xml:",chardata"`
}

type Action struct {
	XMLName xml.Name
	Args []Arg `xml:",any"`
}

type Body struct {
	Action Action `xml:",any"`
}

type Envelope struct {
	XMLName xml.Name `xml:"http://schemas.xmlsoap.org/soap/envelope/ Envelope"`
	EncodingStyle string `xml:"encodingStyle,attr"`
	Body Body `xml:"http://schemas.xmlsoap.org/soap/envelope/ Body"`
}

type Message struct {
	ServiceType string
	Action string
	Args map[string]string
}

func (me *Envelope) Parse() *Message {
	if me.EncodingStyle != EncodingStyle {
		panic(fmt.Sprint(me.EncodingStyle, "!=", EncodingStyle))
	}
	return &Message{
		ServiceType: me.Body.Action.XMLName.Space,
		Action: me.Body.Action.XMLName.Local,
		Args: func() (ret map[string]string) {
			ret = make(map[string]string, len(me.Body.Action.Args))
			for _, arg := range me.Body.Action.Args {
				k := arg.XMLName.Local
				if _, ok := ret[k]; ok {
					panic("dupliate argument name: " + k)
				}
				ret[k] = arg.Value
			}
			return
		}(),
	}
}

func (me *Message) Wrap() *Envelope {
	return &Envelope{
		EncodingStyle: EncodingStyle,
		Body: Body{
			Action{
				xml.Name{
					Space: me.ServiceType,
					Local: me.Action,
				},
				func() (ret []Arg) {
					for k, v := range me.Args {
						ret = append(ret, Arg{
							xml.Name{Local: k},
							v,
						})
					}
					return
				}(),
			},
		},
	}
}

/*
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
*/
