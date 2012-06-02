package ssdp

const (
McastAddr = "239.255.255.250:1900"
)
/*
import (
	"bufio"
	"net/http"
	"net/textproto"
)

type Request struct {
	Method string
	Proto string
	Header http.Header
}

func ReadRequest(b *bufio.Reader) (req *Request, err error) {
	tp := textproto.NewReader(b)
	var s string
	if s, err = tp.ReadLine(); err != nil {
		return nil, err
	}
	defer func() {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
	}()

	var f []string
	if f = strings.Split(s, " "); len(f) != 3 {
		return nil, &badStringError{"malformed request line", s}
	}
	if f[1] != "*" {
		return nil, &badStringError{"bad URL request", f[1]}
	}
	req = &Request{
		Method: f[0],
		Proto: f[2],
	}

	mimeHeader, err := tp.ReadMIMEHeader()
	if err != nil {
		return nil, err
	}
	req.Header = Header(mimeHeader)
}
*/

