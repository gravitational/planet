package main

import (
	"net"
	"net/http"

	"github.com/gravitational/roundtrip"
	"github.com/gravitational/trace"
)

func getClient(path string) (*roundtrip.Client, error) {
	clt, err := roundtrip.NewClient("http://cube", "v1",
		roundtrip.HTTPClient(
			&http.Client{
				Transport: &http.Transport{
					Dial: func(network, addr string) (net.Conn, error) {
						return net.Dial("unix", serverSockPath(path))
					},
				},
			}))
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return clt, nil
}
