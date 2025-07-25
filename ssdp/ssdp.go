package ssdp

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"github.com/anacrolix/log"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

const (
	AddrString    = "239.255.255.250:1900"
	AddrString6LL = "[ff02::c]:1900"
	AddrString6SL = "[ff05::c]:1900"
	rootDevice    = "upnp:rootdevice"
	aliveNTS      = "ssdp:alive"
	byebyeNTS     = "ssdp:byebye"
	mxMax         = 10
)

var NetAddr *net.UDPAddr
var NetAddr6LL *net.UDPAddr
var NetAddr6SL *net.UDPAddr
var AddrString2NetAdd map[string]*net.UDPAddr = make(map[string]*net.UDPAddr, 3)

func init() {
	var err error
	NetAddr, err = net.ResolveUDPAddr("udp4", AddrString)
	if err != nil {
		log.Printf("Could not resolve %s: %s", AddrString, err)
	}
	NetAddr6LL, err = net.ResolveUDPAddr("udp6", AddrString6LL)
	if err != nil {
		log.Printf("Could not resolve %s: %s", AddrString6LL, err)
	}
	NetAddr6SL, err = net.ResolveUDPAddr("udp6", AddrString6SL)
	if err != nil {
		log.Printf("Could not resolve %s: %s", AddrString6SL, err)
	}
	AddrString2NetAdd[AddrString] = NetAddr
	AddrString2NetAdd[AddrString6LL] = NetAddr6LL
	AddrString2NetAdd[AddrString6SL] = NetAddr6SL
}

type badStringError struct {
	what string
	str  string
}

func (e *badStringError) Error() string { return fmt.Sprintf("%s %q", e.what, e.str) }

func ReadRequest(b *bufio.Reader) (req *http.Request, err error) {
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
	if f = strings.SplitN(s, " ", 3); len(f) < 3 {
		return nil, &badStringError{"malformed request line", s}
	}
	if f[1] != "*" {
		return nil, &badStringError{"bad URL request", f[1]}
	}
	req = &http.Request{
		Method: f[0],
	}
	var ok bool
	if req.ProtoMajor, req.ProtoMinor, ok = http.ParseHTTPVersion(strings.TrimSpace(f[2])); !ok {
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
	conn           *net.UDPConn
	Interface      net.Interface
	AddrString     string
	NetAddr        *net.UDPAddr
	Server         string
	Services       []string
	Devices        []string
	IPFilter       func(net.IP) bool
	Location       func(net.IP) string
	UUID           string
	NotifyInterval time.Duration
	closed         chan struct{}
	Logger         log.Logger
}

func makeConn(ifi net.Interface, netAddr *net.UDPAddr) (ret *net.UDPConn, err error) {
	ret, err = net.ListenMulticastUDP("udp", &ifi, netAddr)
	if err != nil {
		return
	}
	if netAddr.IP.String() == AddrString {
		p := ipv4.NewPacketConn(ret)
		if err := p.SetMulticastTTL(2); err != nil {
			log.Print(err)
		}
	} else {
		p := ipv6.NewPacketConn(ret)
		if err := p.SetMulticastHopLimit(2); err != nil {
			log.Print(err)
		}
	}
	// if err := p.SetMulticastLoopback(true); err != nil {
	// 	log.Println(err)
	// }
	return
}

func (me *Server) serve() {
	for {
		size := me.Interface.MTU
		if size > 65536 {
			size = 65536
		} else if size <= 0 { // fix for windows with mtu 4gb
			size = 65536
		}
		b := make([]byte, size)
		n, addr, err := me.conn.ReadFromUDP(b)
		select {
		case <-me.closed:
			return
		default:
		}
		if err != nil {
			me.Logger.Printf("error reading from UDP socket: %s", err)
			break
		}
		go me.handle(b[:n], addr)
	}
}

func (me *Server) Init() (err error) {
	me.closed = make(chan struct{})
	me.conn, err = makeConn(me.Interface, me.NetAddr)
	if me.IPFilter == nil {
		me.IPFilter = func(net.IP) bool { return true }
	}
	return
}

func (me *Server) Close() {
	close(me.closed)
	me.sendByeBye()
	me.conn.Close()
}

func (me *Server) Serve() (err error) {
	go me.serve()
	for {
		select {
		case <-me.closed:
			return
		default:
		}

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
			if !me.IPFilter(ip) {
				continue
			}
			if ip.IsLinkLocalUnicast() {
				// These addresses seem to confuse VLC. Possibly there's supposed to be a zone
				// included in the address, but I don't see one.
				continue
			}
			extraHdrs := [][2]string{
				{"CACHE-CONTROL", fmt.Sprintf("max-age=%d", 5*me.NotifyInterval/2/time.Second)},
				{"LOCATION", me.Location(ip)},
			}
			me.notifyAll(aliveNTS, extraHdrs)
		}
		time.Sleep(me.NotifyInterval)
	}
}

func (me *Server) usnFromTarget(target string) string {
	if target == me.UUID {
		return target
	}
	return me.UUID + "::" + target
}

func (me *Server) makeNotifyMessage(target, nts string, extraHdrs [][2]string) []byte {
	lines := [...][2]string{
		{"HOST", me.AddrString},
		{"NT", target},
		{"NTS", nts},
		{"SERVER", me.Server},
		{"USN", me.usnFromTarget(target)},
	}
	buf := &bytes.Buffer{}
	fmt.Fprint(buf, "NOTIFY * HTTP/1.1\r\n")
	writeHdr := func(keyValue [2]string) {
		fmt.Fprintf(buf, "%s: %s\r\n", keyValue[0], keyValue[1])
	}
	for _, pair := range lines {
		writeHdr(pair)
	}
	for _, pair := range extraHdrs {
		writeHdr(pair)
	}
	fmt.Fprint(buf, "\r\n")
	return buf.Bytes()
}

func (me *Server) send(buf []byte, addr *net.UDPAddr) {
	if n, err := me.conn.WriteToUDP(buf, addr); err != nil {
		me.Logger.Printf("error writing to UDP socket: %s", err)
	} else if n != len(buf) {
		me.Logger.Printf("short write: %d/%d bytes", n, len(buf))
	}
}

func (me *Server) delayedSend(delay time.Duration, buf []byte, addr *net.UDPAddr) {
	go func() {
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
			me.send(buf, addr)
		case <-me.closed:
			if !timer.Stop() {
				<-timer.C
			}
		}
	}()
}

func (me *Server) log(args ...interface{}) {
	args = append([]interface{}{me.Interface.Name + ":"}, args...)
	me.Logger.Print(args...)
}

func (me *Server) sendByeBye() {
	for _, type_ := range me.allTypes() {
		buf := me.makeNotifyMessage(type_, byebyeNTS, nil)
		me.send(buf, me.NetAddr)
	}
}

func (me *Server) notifyAll(nts string, extraHdrs [][2]string) {
	for _, type_ := range me.allTypes() {
		buf := me.makeNotifyMessage(type_, nts, extraHdrs)
		delay := time.Duration(rand.Int63n(int64(100 * time.Millisecond)))
		me.delayedSend(delay, buf, me.NetAddr)
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
		me.Logger.Println(err)
		return
	}
	if req.Method != "M-SEARCH" || req.Header.Get("man") != `"ssdp:discover"` {
		return
	}
	var mx int64
	if req.Header.Get("Host") == me.AddrString {
		mxHeader := req.Header.Get("mx")
		i, err := strconv.ParseUint(mxHeader, 0, 0)
		if err != nil {
			me.Logger.Printf("Invalid mx header %q: %s", mxHeader, err)
			return
		}
		mx = int64(i)
	}
	// fix mx
	if mx <= 0 {
		mx = 1
	}
	if mx > mxMax {
		mx = mxMax
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
			resp := me.makeResponse(ip, type_, req)
			delay := time.Duration(rand.Int63n(int64(time.Second) * mx))
			me.delayedSend(delay, resp, sender)
		}
	}
}

func (me *Server) makeResponse(ip net.IP, targ string, req *http.Request) (ret []byte) {
	resp := &http.Response{
		StatusCode: 200,
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		Request:    req,
	}
	for _, pair := range [...][2]string{
		{"CACHE-CONTROL", fmt.Sprintf("max-age=%d", 5*me.NotifyInterval/2/time.Second)},
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
