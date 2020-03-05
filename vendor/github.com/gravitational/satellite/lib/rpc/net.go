package rpc

import (
	"fmt"
	"net"

	"github.com/gravitational/trace"
)

// SplitHostPort splits the specified host:port into host/port parts.
// If the input address does not have a port, defaultPort as appended.
// Returns the extracted IP address and port
func SplitHostPort(hostPort string, defaultPort int) (addr *net.TCPAddr, err error) {
	_, _, err = net.SplitHostPort(hostPort)
	if ae, ok := err.(*net.AddrError); ok && ae.Err == "missing port in address" {
		hostPort = fmt.Sprintf("%s:%d", hostPort, defaultPort)
		_, _, err = net.SplitHostPort(hostPort)
	}
	if err != nil {
		return nil, trace.Wrap(err)
	}
	addr, err = net.ResolveTCPAddr("tcp", hostPort)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return addr, nil
}
