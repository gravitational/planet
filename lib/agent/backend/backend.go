package backend

import pb "github.com/gravitational/planet/lib/agent/proto/agentpb"

// Backend is an interface that allows persisting health status information
// on a per-node basis.
type Backend interface {
	// UpdateNode updates status for the specified node.
	UpdateNode(status *pb.NodeStatus)

	// Close will clear the state of the backend.
	Close() error
}
