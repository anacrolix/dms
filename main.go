package main

import (
	"bufio"
	"bytes"
	//"crypto/rand"
	"encoding/xml"
	"math/rand"
	//"io/ioutil"
	"fmt"
	//"io"
	"bitbucket.org/anacrolix/dms/ffmpeg"
	"bitbucket.org/anacrolix/dms/soap"
	"bitbucket.org/anacrolix/dms/ssdp"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	serverField         = "Linux/3.4 DLNADOC/1.50 UPnP/1.0 DMS/1.0"
	rootDeviceType      = "urn:schemas-upnp-org:device:MediaServer:1"
	rootDeviceModelName = "dms 1.0"
	resPath             = "/res"
	rootDescPath        = "/rootDesc.xml"
	rootDevice          = "upnp:rootdevice"
	maxAge              = "30"
)

//;DLNA.ORG_FLAGS=017000 00000000000000000000000000000000

func makeDeviceUuid() string {
	/*
		buf := make([]byte, 16)
		if _, err := io.ReadFull(rand.Reader, buf); err != nil {
			panic(err)
		}
	*/
	var buf [16]byte
	return fmt.Sprintf("uuid:%x-%x-%x-%x-%x", buf[:4], buf[4:6], buf[6:8], buf[8:10], buf[10:])
}

type specVersion struct {
	Major int `xml:"major"`
	Minor int `xml:"minor"`
}

type icon struct {
	Mimetype, Width, Height, Depth, URL string
}

type service struct {
	XMLName     xml.Name `xml:"service"`
	ServiceType string   `xml:"serviceType"`
	ServiceId   string   `xml:"serviceId"`
	SCPDURL     string
	ControlURL  string `xml:"controlURL"`
	EventSubURL string `xml:"eventSubURL"`
}

type device struct {
	DeviceType   string `xml:"deviceType"`
	FriendlyName string `xml:"friendlyName"`
	Manufacturer string `xml:"manufacturer"`
	ModelName    string `xml:"modelName"`
	UDN          string
	IconList     []icon
	ServiceList  []service `xml:"serviceList>service"`
}

var services = []service{
	service{
		ServiceType: "urn:schemas-upnp-org:service:ContentDirectory:1",
		ServiceId:   "urn:upnp-org:serviceId:ContentDirectory",
		SCPDURL:     "/scpd/ContentDirectory.xml",
		ControlURL:  "/ctl/ContentDirectory",
	},
	service{
		ServiceType: "urn:schemas-upnp-org:service:ConnectionManager:1",
		ServiceId:   "urn:upnp-org:serviceId:ConnectionManager",
		SCPDURL:     "/scpd/ConnectionManager.xml",
		ControlURL:  "/ctl/ConnectionManager",
	},
}

type root struct {
	XMLName     xml.Name    `xml:"urn:schemas-upnp-org:device-1-0 root"`
	SpecVersion specVersion `xml:"specVersion"`
	Device      device      `xml:"device"`
}

func usnFromTarget(target string) string {
	if target == rootDeviceUUID {
		return target
	}
	return rootDeviceUUID + "::" + target
}

func devices() []string {
	return []string{
		"urn:schemas-upnp-org:device:MediaServer:1",
	}
}

func serviceTypes() (ret []string) {
	for _, s := range services {
		ret = append(ret, s.ServiceType)
	}
	return
}

func targets() (ret []string) {
	for _, a := range [][]string{
		{rootDevice, rootDeviceUUID},
		devices(),
		serviceTypes(),
	} {
		ret = append(ret, a...)
	}
	return
}

func httpPort() int {
	return httpConn.Addr().(*net.TCPAddr).Port
}

func makeNotifyMessage(locHost net.IP, target, nts string) []byte {
	lines := [...][2]string{
		{"HOST", ssdpAddr.String()},
		{"CACHE-CONTROL", "max-age=30"},
		{"LOCATION", fmt.Sprintf("http://%s:%d"+rootDescPath, locHost.String(), httpPort())},
		{"NT", target},
		{"NTS", nts},
		{"SERVER", serverField},
		{"USN", usnFromTarget(target)},
	}
	buf := &bytes.Buffer{}
	fmt.Fprint(buf, "NOTIFY * HTTP/1.1\r\n")
	for _, pair := range lines {
		fmt.Fprintf(buf, "%s: %s\r\n", pair[0], pair[1])
	}
	fmt.Fprint(buf, "\r\n")
	return buf.Bytes()
}

func notifyAlive(conn *net.UDPConn, host net.IP) {
	for _, target := range targets() {
		go func(target string) {
			time.Sleep(time.Duration(rand.Int63n(int64(100 * time.Millisecond))))
			data := makeNotifyMessage(host, target, "ssdp:alive")
			n, err := conn.WriteToUDP(data, ssdpAddr)
			ssdpLogger.Println("sending", string(data))
			if err != nil {
				panic(err)
			}
			if n != len(data) {
				panic(fmt.Sprintf("sent %d < %d bytes", n, len(data)))
			}
		}(target)
	}
}

func serveHTTP() {
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Printf("got http request: %#v", r)
			w.Header().Set("Ext", "")
			w.Header().Set("Server", serverField)
			http.DefaultServeMux.ServeHTTP(w, r)
		}),
	}
	if err := srv.Serve(httpConn); err != nil {
		panic(err)
	}
	panic(nil)
}

func sSDPInterface(if_ net.Interface) {
	conn, err := net.ListenMulticastUDP("udp4", &if_, ssdpAddr)
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	f, err := conn.File()
	if err != nil {
		panic(err)
	}
	fd := int(f.Fd())
	if err := syscall.SetsockoptInt(fd, syscall.SOL_IP, syscall.IP_MULTICAST_TTL, 4); err != nil {
		panic(err)
	}
	f.Close()
	go func() {
		b := make([]byte, if_.MTU)
		for {
			n, retAddr, err := conn.ReadFromUDP(b)
			if err != nil {
				log.Println(err)
				continue
			}
			req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(b[:n])))
			if err != nil {
				log.Println(err)
				continue
			}
			if req.Method != "M-SEARCH" || req.Header.Get("man") != `"ssdp:discover"` {
				continue
			}
			var mx uint
			if req.Host == ssdp.McastAddr {
				i, err := strconv.ParseUint(req.Header.Get("mx"), 0, 0)
				if err != nil {
					log.Println(err)
					continue
				}
				mx = uint(i)
			} else {
				mx = 1
			}
			for respTarg, count := range func(st string) (ret map[string]int) {
				if st == "ssdp:all" {
					ret := make(map[string]int)
					ret[rootDevice] = 3
					for _, d := range devices() {
						ret[d] = 2
					}
					for _, k := range serviceTypes() {
						ret[k] = 1
					}
				}
				for _, t := range targets() {
					if st == t {
						ret = map[string]int{
							t: 1,
						}
						break
					}
				}
				return
			}(req.Header.Get("st")) {
				addrs, err := if_.Addrs()
				if err != nil {
					panic(err)
				}
				for _, addr := range addrs {
					var addrIP net.IP
					if ipnet, ok := addr.(*net.IPNet); ok {
						addrIP = ipnet.IP
					} else {
						panic(addr)
					}
					resp := &http.Response{
						StatusCode: 200,
						ProtoMajor: 1,
						ProtoMinor: 1,
						Request:    req,
						Header:     make(http.Header),
					}
					url := url.URL{
						Scheme: "http",
						Host: (&net.TCPAddr{
							IP:   addrIP,
							Port: httpPort(),
						}).String(),
						Path: rootDescPath,
					}
					for _, pair := range [...][2]string{
						{"CACHE-CONTROL", "max-age=" + maxAge},
						{"EXT", ""},
						{"LOCATION", url.String()},
						{"SERVER", serverField},
						{"ST", respTarg},
						{"USN", usnFromTarget(respTarg)},
					} {
						resp.Header.Set(pair[0], pair[1])
					}
					buf := &bytes.Buffer{}
					if err := resp.Write(buf); err != nil {
						panic(err)
					}
					for i := 0; i < count; i++ {
						go func() {
							time.Sleep(time.Duration(rand.Int63n(int64(time.Second) * int64(mx))))
							ssdpLogger.Printf("%s -> %s\n%s", if_, retAddr, buf.String())
							n, err := conn.WriteToUDP(buf.Bytes(), retAddr)
							if err != nil {
								panic(err)
							}
							if n != len(buf.Bytes()) {
								panic(fmt.Sprint(n, len(buf.Bytes())))
							}
						}()
					}
				}
			}
		}
	}()
	for {
		addrs, err := if_.Addrs()
		if err != nil {
			panic(err)
		}
		for _, addr := range addrs {
			addr4 := addr.(*net.IPNet).IP.To4()
			if addr4 == nil {
				continue
			}
			notifyAlive(conn, addr4)
		}
		time.Sleep(10 * time.Second)
	}
}

func doSSDP() {
	active := map[int]bool{}
	for {
		ifs, err := net.Interfaces()
		if err != nil {
			panic(err)
		}
		for _, if_ := range ifs {
			if active[if_.Index] {
				continue
			}
			active[if_.Index] = true
			go sSDPInterface(if_)
		}
		time.Sleep(time.Second)
	}
}

var (
	rootDeviceUUID string
	httpConn       *net.TCPListener
	ssdpAddr       *net.UDPAddr
	ssdpLogger     *log.Logger
	rootDescXML    []byte
)

func childCount(path_ string) (uint, error) {
	f, err := os.Open(path_)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	names, err := f.Readdirnames(-1)
	if err != nil {
		return 0, err
	}
	return uint(len(names)), nil
}

func durationToDIDLString(d time.Duration) string {
	ns := d % time.Second
	d /= time.Second
	s := d % 60
	d /= 60
	m := d % 60
	d /= 60
	h := d
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%d:%02d:%02d.%09d", h, m, s, ns), "0"), ".")
}

func itemResExtra(path string) (bitrate uint, duration string) {
	info, err := ffmpeg.Probe(path)
	if err != nil {
		log.Printf("error probing %s: %s", path, err)
		return
	}
	if _, err = fmt.Sscan(info.Format["bit_rate"], &bitrate); err != nil {
		panic(err)
	}
	var f float64
	if _, err = fmt.Sscan(info.Format["duration"], &f); err != nil {
		panic(err)
	}
	duration = durationToDIDLString(time.Duration(f * float64(time.Second)))
	return
}

func ReadContainer(path_, parentID, host string) (ret []UPNPObject) {
	dir, err := os.Open(path_)
	if err != nil {
		panic(err)
	}
	defer dir.Close()
	fis, err := dir.Readdir(-1)
	if err != nil {
		panic(err)
	}
	for _, fi := range fis {
		obj := UPNPObject{
			ID:         path.Join(path_, fi.Name()),
			Title:      fi.Name(),
			Restricted: 1,
			ParentID:   parentID,
		}
		if fi.IsDir() {
			obj.XMLName.Local = "container"
			childCount, err := childCount(path.Join(path_, fi.Name()))
			if err != nil {
				log.Println(err)
			} else {
				obj.ChildCount = childCount
			}
			obj.Class = "object.container.storageFolder"
		} else {
			mimeType := mime.TypeByExtension(path.Ext(fi.Name()))
			obj.XMLName.Local = "item"
			obj.Class = "object.item." + strings.SplitN(mimeType, "/", 2)[0] + "Item"
			values := url.Values{}
			values.Set("path", path.Join(path_, fi.Name()))
			url_ := &url.URL{
				Scheme:   "http",
				Host:     host,
				Path:     resPath,
				RawQuery: values.Encode(),
			}
			mainRes := Resource{
				ProtocolInfo: "http-get:*:" + mimeType + ":DLNA.ORG_OP=01;DLNA.ORG_CI=0;DLNA.ORG_FLAGS=017000 00000000000000000000000000000000",
				URL:          url_.String(),
				Size:         uint64(fi.Size()),
			}
			mainRes.Bitrate, mainRes.Duration = itemResExtra(path.Join(path_, fi.Name()))
			obj.Res = append(obj.Res, mainRes)
		}

		ret = append(ret, obj)
	}
	return
}

func main() {
	rootDeviceUUID = makeDeviceUuid()
	var err error
	rootDescXML, err = xml.MarshalIndent(
		root{
			SpecVersion: specVersion{Major: 1, Minor: 0},
			Device: device{
				DeviceType: rootDeviceType,
				FriendlyName: fmt.Sprintf("%s: %s on %s", rootDeviceModelName, func() string {
					user, err := user.Current()
					if err != nil {
						panic(err)
					}
					return user.Name
				}(),
					func() string {
						name, err := os.Hostname()
						if err != nil {
							panic(err)
						}
						return name
					}()),
				Manufacturer: "Matt Joiner <anacrolix@gmail.com>",
				ModelName:    rootDeviceModelName,
				UDN:          rootDeviceUUID,
				ServiceList:  services,
			},
		},
		" ", "  ")
	if err != nil {
		panic(err)
	}
	rootDescXML = append([]byte(`<?xml version="1.0"?>`), rootDescXML...)
	log.Println(string(rootDescXML))
	ssdpAddr, err = net.ResolveUDPAddr("udp4", ssdp.McastAddr)
	if err != nil {
		panic(err)
	}
	httpConn, err = net.ListenTCP("tcp", &net.TCPAddr{})
	if err != nil {
		panic(err)
	}
	defer httpConn.Close()
	log.Println("HTTP server on", httpConn.Addr())
	logFile, err := os.Create("ssdp.log")
	if err != nil {
		panic(err)
	}
	defer logFile.Close()
	ssdpLogger = log.New(logFile, "", log.Ltime|log.Lmicroseconds)
	http.HandleFunc("/", func(http.ResponseWriter, *http.Request) {
		panic(nil)
	})
	http.HandleFunc(resPath, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			log.Println(err)
		}
		path := r.Form.Get("path")
		http.ServeFile(w, r, path)
	})
	http.HandleFunc(rootDescPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", `text/xml; charset="utf-8"`)
		w.Header().Set("content-length", fmt.Sprint(len(rootDescXML)))
		w.Header().Set("server", serverField)
		w.Write(rootDescXML)
	})
	http.HandleFunc("/ctl/ContentDirectory", func(w http.ResponseWriter, r *http.Request) {
		var env soap.Envelope
		if err := xml.NewDecoder(r.Body).Decode(&env); err != nil {
			panic(err)
		}
		msg := env.Parse()
		serviceTypeParts := strings.Split(msg.ServiceType, ":")
		if serviceTypeParts[len(serviceTypeParts)-2] != "ContentDirectory" {
			panic(serviceTypeParts)
		}
		rmsg := soap.Message{
			ServiceType: msg.ServiceType,
			Action:      msg.Action + "Response",
		}
		switch msg.Action {
		case "GetSortCapabilities":
			rmsg.Args = map[string]string{
				"SortCaps": "dc:title",
			}
		case "Browse":
			log.Println("browse args", msg)
			path := msg.Args["ObjectID"]
			if path == "0" {
				path = "/mnt/data/towatch"
			}
			switch msg.Args["BrowseFlag"] {
			case "BrowseDirectChildren":
				startingIndex, _ := strconv.ParseUint(msg.Args["StartingIndex"], 0, 64)
				requestedCount, _ := strconv.ParseUint(msg.Args["RequestedCount"], 0, 64)
				objs := ReadContainer(path, msg.Args["ObjectID"], r.Host)
				totalMatches := len(objs)
				objs = objs[startingIndex:]
				if requestedCount != 0 && int(requestedCount) < len(objs) {
					objs = objs[:requestedCount]
				}
				result, err := xml.MarshalIndent(objs, "", "  ")
				if err != nil {
					panic(err)
				}
				rmsg.Args = map[string]string{
					"TotalMatches":   fmt.Sprintf("%d", totalMatches),
					"NumberReturned": fmt.Sprintf("%d", len(objs)),
					"Result":         string(didl_lite(string(result))),
				}
			default:
				panic(nil)
			}
		default:
			panic(msg.Action)
		}
		w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
		w.Header().Set("Ext", "")
		w.Header().Set("Server", serverField)
		body, err := xml.MarshalIndent(rmsg.Wrap(), "", "  ")
		if err != nil {
			panic(err)
		}
		body = append([]byte(xml.Header), body...)
		log.Println("response:", string(body))
		if _, err := w.Write(body); err != nil {
			panic(err)
		}
	})
	go serveHTTP()
	doSSDP()
}

type Resource struct {
	XMLName      xml.Name `xml:"res"`
	ProtocolInfo string   `xml:"protocolInfo,attr"`
	URL          string   `xml:",chardata"`
	Size         uint64   `xml:"size,attr"`
	Bitrate      uint     `xml:"bitrate,attr"`
	Duration     string   `xml:"duration,attr"`
}

type UPNPObject struct {
	XMLName    xml.Name
	ID         string `xml:"id,attr"`
	ParentID   string `xml:"parentID,attr"`
	Restricted int    `xml:"restricted,attr"` // indicates whether the object is modifiable
	Class      string `xml:"upnp:class"`
	Icon       string `xml:"upnp:icon,omitempty"`
	Title      string `xml:"dc:title"`
	Res        []Resource
	ChildCount uint `xml:"childCount,attr"`
}

func didl_lite(chardata string) string {
	return `<DIDL-Lite` +
		` xmlns:dc="http://purl.org/dc/elements/1.1/"` +
		` xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/"` +
		` xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/"` +
		` xmlns:dlna="urn:schemas-dlna-org:metadata-1-0/">` +
		chardata +
		`</DIDL-Lite>`
}
