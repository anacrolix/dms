package main

import (
	"crypto/rand"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
)

func makeDeviceUuid() string {
	buf := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		panic(err)
	}
	return fmt.Sprintf("uuid:%x-%x-%x-%x-%x", buf[:4], buf[4:6], buf[6:8], buf[8:10], buf[10:])
}

type server struct {
	uuid     string
	xmlDesc  []byte
	ssdpConn *net.UDPConn
	ssdpAddr *net.UDPAddr
}

type devDescRoot struct {
	XMLName xml.Name `xml:"root"`
	Device  device   `xml:"device"`
}

type device struct {
	UDN string
}

func respondToSSDP(conn *net.UDPConn) {
	for {
		b := make([]byte, 4096)
		n, addr, err := conn.ReadFromUDP(b)
		log.Println("received ssdp:", n, addr, err, string(b))
	}
}

func main() {
	s := server{
		uuid: makeDeviceUuid(),
	}
	var err error
	s.xmlDesc, err = xml.MarshalIndent(devDescRoot{Device: device{UDN: s.uuid}}, " ", "  ")
	if err != nil {
		panic(err)
	}
	log.Println("device description:", string(s.xmlDesc))
	go func() {
		if err := http.ListenAndServe(":0", nil); err != nil {
			panic(err)
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
	respondToSSDP(s.ssdpConn)
}
