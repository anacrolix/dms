// +build !windows

package ssdp

import (
	"net"
	"syscall"
)

func setTTL(conn *net.UDPConn, ttl int) error {
	f, err := conn.File()
	if err != nil {
		return err
	}
	defer f.Close()
	fd := int(f.Fd())
	return syscall.SetsockoptInt(fd, syscall.SOL_IP, syscall.IP_MULTICAST_TTL, ttl)
}
