package backend

import "github.com/gravitational/planet/lib/agent/health"

type Backend interface {
	// Add status for the specified node.
	AddStats(status *health.NodeStatus)

	// Clear the state of the backend.
	Close() error
}
