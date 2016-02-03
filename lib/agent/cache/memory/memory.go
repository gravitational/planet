package memory

import (
	"sync"

	"github.com/gravitational/planet/lib/agent/backend"
	pb "github.com/gravitational/planet/lib/agent/proto/agentpb"
)

// cache implements in-memory agent.Cache that replicates into the
// specified backend.
type cache struct {
	backend backend.Backend
	mu      sync.RWMutex // protects following attributes
	*pb.SystemStatus
}

// New returns a new instance of cache specialized to use given backend.
func New(backend backend.Backend) *cache {
	return &cache{
		backend:      backend,
		SystemStatus: &pb.SystemStatus{Status: pb.SystemStatus_Unknown},
	}
}

// Update updates the system status in cache.
func (r *cache) Update(status *pb.SystemStatus) error {
	r.mu.Lock()
	r.SystemStatus = status.Clone()
	r.mu.Unlock()
	return r.backend.Update(status)
}

// Update updates the specified node status in cache.
func (r *cache) UpdateNode(status *pb.NodeStatus) error {
	var found bool
	r.mu.Lock()
	for i, node := range r.Nodes {
		if node.Name == status.Name {
			r.Nodes[i] = status.Clone()
			found = true
			break
		}
	}
	if !found {
		r.Nodes = append(r.Nodes, status.Clone())
	}
	r.mu.Unlock()
	return r.backend.UpdateNode(status)
}

// RecentNodeStatus obtains the last known status of the specified node.
func (r *cache) RecentNodeStatus(name string) (result *pb.NodeStatus, err error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, node := range r.Nodes {
		if node.Name == name {
			return node.Clone(), nil
		}
	}
	memberStatus := &pb.MemberStatus{
		Status: pb.MemberStatus_None,
	}
	return &pb.NodeStatus{
		Name:         name,
		MemberStatus: memberStatus,
		Status:       pb.NodeStatus_Unknown,
	}, nil
}

// RecentNodeStatus obtains the last known cluster status.
func (r *cache) RecentStatus() (*pb.SystemStatus, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.SystemStatus.Clone(), nil
}

// Close is a no-op for in-memory cache.
func (r *cache) Close() error {
	return nil
}
