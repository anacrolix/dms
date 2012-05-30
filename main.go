package main

import (
	"bytes"
	//"crypto/rand"
	"encoding/xml"
	//"io/ioutil"
	"fmt"
	//"io"
	"bitbucket.org/anacrolix/dms/soap"
	"log"
	"net"
	"net/http"
	"os"
	"os/user"
	"strings"
	"syscall"
	"time"
)

const (
	serverField         = "Linux/3.4 UPnP/1.1 DMS/1.0"
	rootDeviceType      = "urn:schemas-upnp-org:device:MediaServer:1"
	rootDeviceModelName = "dms 1.0"
)

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

func targets() []string {
	return append([]string{
		"upnp:rootdevice",
		"urn:schemas-upnp-org:device:MediaServer:1",
		"urn:schemas-upnp-org:service:ContentDirectory:1",
	}, rootDeviceUUID)
}

func httpPort() int {
	return httpConn.Addr().(*net.TCPAddr).Port
}

func makeNotifyMessage(locHost net.IP, target, nts string) []byte {
	lines := [...][2]string{
		{"HOST", ssdpAddr.String()},
		{"CACHE-CONTROL", "max-age = 30"},
		{"LOCATION", fmt.Sprintf("http://%s:%d/rootDesc.xml", locHost.String(), httpPort())},
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
		data := makeNotifyMessage(host, target, "ssdp:alive")
		n, err := conn.WriteToUDP(data, ssdpAddr)
		ssdpLogger.Println("sending", string(data))
		if err != nil {
			panic(err)
		}
		if n != len(data) {
			panic(fmt.Sprintf("sent %d < %d bytes", n, len(data)))
		}
	}
}

func serveHTTP() {
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Println("got http request:", r)
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
			log.Println(addr)
			notifyAlive(conn, addr4)
		}
		time.Sleep(time.Second)
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
	ssdpAddr, err = net.ResolveUDPAddr("udp4", "239.255.255.250:1900")
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
	http.HandleFunc("/rootDesc.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", `text/xml; charset="utf-8"`)
		w.Header().Set("content-length", fmt.Sprint(len(rootDescXML)))
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
				path = "/"
			}
			dir, err := os.Open(path)
			if err != nil {
				panic(err)
			}
			names, err := dir.Readdirnames(-1)
			if err != nil {
				panic(err)
			}
			result, err := xml.MarshalIndent(func() (ret []UPNPObject) {
				for _, n := range names {
					ret = append(ret, UPNPObject{
						XMLName:    xml.Name{Local: "item"},
						ID:         n,
						ParentID:   msg.Args["ObjectID"],
						Restricted: 1,
						Class:      "object.item.videoItem",
						Title:      n,
					})
				}
				return
			}(), "", "  ")
			if err != nil {
				panic(err)
			}
			rmsg.Args = map[string]string{
				"TotalMatches":   fmt.Sprintf("%d", len(names)),
				"NumberReturned": fmt.Sprintf("%d", len(names)),
				"Result":         string(didl_lite(string(result))),
			}
		default:
			panic(msg.Action)
		}
		w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
		body, err := xml.MarshalIndent(rmsg.Wrap(), "", "  ")
		if err != nil {
			panic(err)
		}
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
	ProtocolInfo string   `xml:"protocolInfo"`
	URL          string   `xml:",chardata"`
	Size         int      `xml:"size,attr"`
	Bitrate      int      `xml:"bitrate,attr"`
	Duration     string   `xml:"duration,attr"`
}

type UPNPObject struct {
	XMLName    xml.Name
	ID         string `xml:"id,attr"`
	ParentID   string `xml:"parentID,attr"`
	Restricted int    `xml:"restricted,attr"`
	Class      string `xml:"upnp:class"`
	Icon       string `xml:"upnp:icon"`
	Title      string `xml:"dc:title"`
	Res        []Resource
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
