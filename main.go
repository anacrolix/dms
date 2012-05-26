package main

import (
	"bytes"
	"crypto/rand"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"syscall"
	"time"
)

const (
	serverField   = "Linux/3.4 UPnP/1.1 DMS/1.0"
	ssdpMcastAddr = "239.255.255.250:1900"
	httpPort      = "1337"
	rootDeviceType = "urn:schemas-upnp-org:device:MediaServer:1"
)

func makeDeviceUuid() string {
	buf := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		panic(err)
	}
	return fmt.Sprintf("uuid:%x-%x-%x-%x-%x", buf[:4], buf[4:6], buf[6:8], buf[8:10], buf[10:])
}

type server struct {
	uuid       string
	xmlDesc    []byte
	ssdpConn   *net.UDPConn
	ssdpAddr   *net.UDPAddr
	http       *http.Server
	ssdpLogger *log.Logger
}

type specVersion struct {
	Major int
	Minor int
}

type icon struct {
	Mimetype, Width, Height, Depth, URL string
}

type service struct {
	ServiceType, ServiceId, SCPDURL, ControlURL, EventSubURL string
}

type device struct {
	DeviceType, FriendlyName, Manufacturer, ModelName, UDN string
	IconList []icon
	ServiceList []service
}

var services = []service{
	service{
		ServiceType: "urn:schemas-upnp-org:service:ContentDirectory:1",
		ServiceId: "urn:upnp-org:serviceId:ContentDirectory",
		SCPDURL: "/scpd/ContentDirectory.xml",
		ControlURL: "/ctl/ContentDirectory",
		EventSubURL: "/evt/ContentDirectory",
	},
}

type root struct {
	Device device
	SpecVersion specVersion
}

func respondToSSDP(conn *net.UDPConn, lgr *log.Logger) {
	for {
		b := make([]byte, 4096)
		n, addr, err := conn.ReadFromUDP(b)
		lgr.Println("received ssdp:", n, addr, err, string(b))
	}
}

func usnFromTarget(target, uuid string) string {
	if target == uuid {
		return target
	}
	return uuid + "::" + target
}

func (me *server) notifyAlive() {
	conn := me.ssdpConn
	uuid := me.uuid
	logger := me.ssdpLogger
	for {
		for _, target := range me.targets() {
			lines := [...][2]string{
				{"HOST", ssdpMcastAddr},
				{"CACHE-CONTROL", "max-age = 30"},
				{"LOCATION", "http://192.168.26.2:" + httpPort + "/rootDesc.xml"},
				{"NT", target},
				{"NTS", "ssdp:alive"},
				{"SERVER", serverField},
				{"USN", usnFromTarget(target, uuid)},
			}
			buf := &bytes.Buffer{}
			fmt.Fprint(buf, "NOTIFY * HTTP/1.1\r\n")
			for _, pair := range lines {
				fmt.Fprintf(buf, "%s: %s\r\n", pair[0], pair[1])
			}
			fmt.Fprint(buf, "\r\n")
			n, err := conn.WriteToUDP(buf.Bytes(), me.ssdpAddr)
			logger.Println("sending", string(buf.Bytes()))
			if err != nil {
				panic(err)
			}
			if n != buf.Len() {
				panic(fmt.Sprintf("sent %d < %d bytes", n, buf.Len()))
			}
		}
		time.Sleep(time.Second)
	}
}

func (me *server) targets() (ret []string) {
	ret = append([]string{
		"upnp:rootdevice",
		"urn:schemas-upnp-org:device:MediaServer:1",
	}, me.uuid)
	return
}

func main() {
	s := server{
		uuid: makeDeviceUuid(),
	}
	ssdpLogFile, err := os.Create("ssdp.log")
	if err != nil {
		log.Fatalln(err)
	}
	s.ssdpLogger = log.New(ssdpLogFile, "", log.Flags())
	s.xmlDesc, err = xml.MarshalIndent(root{Device: device{UDN: s.uuid, ServiceList:services}}, " ", "  ")
	if err != nil {
		panic(err)
	}
	log.Println("device description:", string(s.xmlDesc))
	go func() {
		if err := http.ListenAndServe(":"+httpPort, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Println("got http request:", r)
			http.NotFound(w, r)
		})); err != nil {
			log.Fatalln(err)
		}
	}()
	s.ssdpAddr, err = net.ResolveUDPAddr("udp4", "239.255.255.250:1900")
	if err != nil {
		panic(err)
	}
	s.ssdpConn, err = net.ListenMulticastUDP("udp4", nil, s.ssdpAddr)
	if err != nil {
		panic(err)
	}
	f, _ := s.ssdpConn.File()
	syscall.SetsockoptInt(int(f.Fd()), syscall.IPPROTO_IP, syscall.IP_MULTICAST_TTL, 4)
	f.Close()
	go s.notifyAlive()
	respondToSSDP(s.ssdpConn, s.ssdpLogger)
}
