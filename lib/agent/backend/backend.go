package backend

import pb "github.com/gravitational/planet/lib/agent/proto/agentpb"

// Backend is an interface that allows to persist health status results
// after a monitoring test run.
type Backend interface {
	// Update updates status for the cluster.
	Update(status *pb.SystemStatus) error

	// UpdateNode updates status for the specified node.
	UpdateNode(status *pb.NodeStatus) error

	// Close resets the backend and releases any resources.
	Close() error
}
