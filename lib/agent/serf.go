package agent

import (
	serf "github.com/hashicorp/serf/client"
)

// serfClient is the minimal interface to the serf cluster.
// It enables mocking access to serf network in tests.
type serfClient interface {
	// Members lists members of the serf cluster.
	Members() ([]serf.Member, error)
	// Stream subcribes the caller to the serf event stream.
	// Filter can be used to restrict the events.
	// Returns an opaque handle to be used with Stop.
	Stream(filter string, eventc chan<- map[string]interface{}) (serf.StreamHandle, error)
	// Stop cancels the serf event delivery and removes the subscription.
	Stop(serf.StreamHandle) error
	// Close closes the client.
	Close() error
	// Join attempts to join an existing serf cluster identified by peers.
	// Replay controls if previous user events are replayed once this node has joined the cluster.
	Join(peers []string, replay bool) (int, error)
}
