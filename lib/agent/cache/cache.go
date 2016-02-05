package cache

import pb "github.com/gravitational/planet/lib/agent/proto/agentpb"

// Cache is an interface that allows access to recent health status information
// collected on a per-node basis.
// All methods are expected to be thread-safe as they might be used from multiple
// goroutines.
type Cache interface {
	// Update system status.
	Update(status *pb.SystemStatus) error

	// Read last known system status.
	RecentStatus() (*pb.SystemStatus, error)

	// Update status for the specified node.
	UpdateNode(status *pb.NodeStatus) error

	// Read last known status records for specified node.
	RecentNodeStatus(node string) (*pb.NodeStatus, error)

	// Close resets the cache and closes any resources.
	Close() error
}
