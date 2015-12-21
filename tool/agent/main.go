package main

import (
	"flag"
	"log"
)

var bindAddr = flag.String("bind-addr", "0.0.0.0:7946", "Address to bind network listeners to.  To use an IPv6 address, specify [::1] or [::1]:7946.")

var rpcAddr = flag.String("rpc-addr", "127.0.0.1:7373", "Address to bind the RPC listener.")

var opMode = flag.String("mode", "", "Mode to operate in (master, node).")

func main() {
	flag.Parse()

	if *opMode == "" {
		*opMode = string(master)
	}

	conf := &config{
		bindAddr: *bindAddr,
		rpcAddr:  *rpcAddr,
		mode:     mode(*opMode),
	}
	_, err := newAgent(conf)
	if err != nil {
		log.Fatalln(err)
	}

	select {}
}
