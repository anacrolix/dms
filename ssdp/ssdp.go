package ssdp

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

const (
	AddrString            = "239.255.255.250:1900"
	rootDevice            = "upnp:rootdevice"
	DefaultNotifyInterval = 30
	aliveNTS              = "ssdp:alive"
)

var (
	NetAddr *net.UDPAddr
)

func init() {
	var err error
	NetAddr, err = net.ResolveUDPAddr("udp4", AddrString)
	if err != nil {
		panic(err)
	}
}

type badStringError struct {
	what string
	str  string
}

func (e *badStringError) Error() string { return fmt.Sprintf("%s %q", e.what, e.str) }

type Request struct {
	Method     string
	ProtoMajor int
	ProtoMinor int
	Header     http.Header
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
	// TODO a split that only allows N values?
	if f = strings.Split(s, " "); len(f) != 3 {
		return nil, &badStringError{"malformed request line", s}
	}
	if f[1] != "*" {
		return nil, &badStringError{"bad URL request", f[1]}
	}
	req = &Request{
		Method: f[0],
	}
	var ok bool
	if req.ProtoMajor, req.ProtoMinor, ok = http.ParseHTTPVersion(f[2]); !ok {
		return nil, &badStringError{"malformed HTTP version", f[2]}
	}

	mimeHeader, err := tp.ReadMIMEHeader()
	if err != nil {
		return nil, err
	}
	req.Header = http.Header(mimeHeader)
	return
}

type Server struct {
	conn      *net.UDPConn
	Interface net.Interface
	Server    string
	Services  []string
	Devices   []string
	Location  func(net.IP) string
	UUID      string
}

func makeConn(ifi net.Interface) (ret *net.UDPConn, err error) {
	ret, err = net.ListenMulticastUDP("udp", &ifi, NetAddr)
	if err != nil {
		return
	}
	if err := setTTL(ret, 2); err != nil {
		log.Println(err)
	}
	return
}

func (me *Server) serve() {
	for {
		b := make([]byte, me.Interface.MTU)
		n, addr, err := me.conn.ReadFromUDP(b)
		if err != nil {
			panic(err)
		}
		go me.handle(b[:n], addr)
	}
}

func (me *Server) Serve() (err error) {
	me.conn, err = makeConn(me.Interface)
	if err != nil {
		return
	}
	defer me.conn.Close()
	go me.serve()
	for {
		addrs, err := me.Interface.Addrs()
		if err != nil {
			return err
		}
		for _, addr := range addrs {
			ip := func() net.IP {
				switch val := addr.(type) {
				case *net.IPNet:
					return val.IP
				case *net.IPAddr:
					return val.IP
				}
				panic(fmt.Sprint("unexpected addr type:", addr))
			}()
			me.notifyAll(ip, aliveNTS)
		}
		time.Sleep(60 * time.Second)
	}
	panic(nil)
}

func (me *Server) usnFromTarget(target string) string {
	if target == me.UUID {
		return target
	}
	return me.UUID + "::" + target
}

func (me *Server) makeNotifyMessage(location, target, nts string) []byte {
	lines := [...][2]string{
		{"HOST", AddrString},
		{"CACHE-CONTROL", "max-age=120"},
		{"LOCATION", location},
		{"NT", target},
		{"NTS", nts},
		{"SERVER", me.Server},
		{"USN", me.usnFromTarget(target)},
	}
	buf := &bytes.Buffer{}
	fmt.Fprint(buf, "NOTIFY * HTTP/1.1\r\n")
	for _, pair := range lines {
		fmt.Fprintf(buf, "%s: %s\r\n", pair[0], pair[1])
	}
	fmt.Fprint(buf, "\r\n")
	return buf.Bytes()
}

func (me *Server) delayedSend(delay time.Duration, buf []byte, addr *net.UDPAddr) {
	time.Sleep(delay)
	n, err := me.conn.WriteToUDP(buf, addr)
	if err != nil {
		panic(err)
	}
	if n != len(buf) {
		panic(err)
	}
}

func (me *Server) notifyAll(ip net.IP, nts string) {
	loc := me.Location(ip)
	for _, type_ := range me.allTypes() {
		buf := me.makeNotifyMessage(loc, type_, nts)
		delay := time.Duration(rand.Int63n(int64(100 * time.Millisecond)))
		go me.delayedSend(delay, buf, NetAddr)
	}
}

func (me *Server) allTypes() (ret []string) {
	for _, a := range [][]string{
		{rootDevice, me.UUID},
		me.Devices,
		me.Services,
	} {
		ret = append(ret, a...)
	}
	return
}

func (me *Server) handle(buf []byte, sender *net.UDPAddr) {
	req, err := ReadRequest(bufio.NewReader(bytes.NewReader(buf)))
	if err != nil {
		log.Println(err)
		return
	}
	if req.Method != "M-SEARCH" || req.Header.Get("man") != `"ssdp:discover"` {
		return
	}
	var mx uint
	if req.Header.Get("Host") == AddrString {
		i, err := strconv.ParseUint(req.Header.Get("mx"), 0, 0)
		if err != nil {
			log.Println(err)
			return
		}
		mx = uint(i)
	} else {
		mx = 1
	}
	types := func(st string) []string {
		if st == "ssdp:all" {
			return me.allTypes()
		}
		for _, t := range me.allTypes() {
			if t == st {
				return []string{t}
			}
		}
		return nil
	}(req.Header.Get("st"))
	for _, ip := range func() (ret []net.IP) {
		addrs, err := me.Interface.Addrs()
		if err != nil {
			panic(err)
		}
		for _, addr := range addrs {
			if ip, ok := func() (net.IP, bool) {
				switch data := addr.(type) {
				case *net.IPNet:
					if data.Contains(sender.IP) {
						return data.IP, true
					}
					return nil, false
				case *net.IPAddr:
					return data.IP, true
				}
				panic(addr)
			}(); ok {
				ret = append(ret, ip)
			}
		}
		return
	}() {
		for _, type_ := range types {
			resp := me.makeResponse(ip, type_)
			delay := time.Duration(rand.Int63n(int64(time.Second) * int64(mx)))
			go me.delayedSend(delay, resp, sender)
		}
	}
}

func (me *Server) makeResponse(ip net.IP, targ string) (ret []byte) {
	resp := &http.Response{
		StatusCode: 200,
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
	}
	for _, pair := range [...][2]string{
		{"CACHE-CONTROL", fmt.Sprintf("max-age=%d", (5*DefaultNotifyInterval)/2)},
		{"EXT", ""},
		{"LOCATION", me.Location(ip)},
		{"SERVER", me.Server},
		{"ST", targ},
		{"USN", me.usnFromTarget(targ)},
	} {
		resp.Header.Set(pair[0], pair[1])
	}
	buf := &bytes.Buffer{}
	if err := resp.Write(buf); err != nil {
		panic(err)
	}
	return buf.Bytes()
}
