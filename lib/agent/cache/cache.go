package cache

import "github.com/gravitational/planet/lib/agent/health"

type Cache interface {
	// Add status for the specified node.
	AddStats(node string, stats *health.NodeStats) error

	// Read status history for the specified node.
	// Stats are returned sorted by time with the latest at the end.
	RecentStats(node string) ([]*health.NodeStats, error)

	// Close will clear the state of the backend.
	Close() error
}
