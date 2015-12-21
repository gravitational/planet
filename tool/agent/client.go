package main

import (
	serfClient "github.com/gravitational/planet/Godeps/_workspace/src/github.com/hashicorp/serf/client"
)

type client struct {
	*serfClient.RPCClient
}
