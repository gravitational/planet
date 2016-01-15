package cache

import pb "github.com/gravitational/planet/lib/agent/proto/agentpb"

type Cache interface {
	// Update status for the specified node.
	UpdateNode(status *pb.NodeStatus) error

	// Read status history for the specified node.
	// Stats are returned sorted by time with the latest at the end.
	RecentStatus(node string) ([]*pb.Probe, error)

	// Close will clear the state of the backend.
	Close() error
}
