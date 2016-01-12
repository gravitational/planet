package backend

import pb "github.com/gravitational/planet/lib/agent/proto/agentpb"

type Backend interface {
	// Update status for the specified node.
	UpdateNode(status *pb.NodeStatus)

	// Clear the state of the backend
	Close() error
}
