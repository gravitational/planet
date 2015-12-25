package monitoring

import (
	"github.com/gravitational/planet/lib/monitoring"
)

type Status struct {
	Status string       `json:"status"`
	Nodes  []NodeStatus `json:"nodes,omitempty"`
}

type NodeStatus struct {
	Name         string `json:"name"`
	SystemStatus monitoring.SystemStatus
}
