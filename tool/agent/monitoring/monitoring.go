package monitoring

import (
	"github.com/gravitational/planet/lib/monitoring"
)

type Status struct {
	Status string       `json:"status"`
	Nodes  []NodeStatus `json:"nodes"`
}

type NodeStatus struct {
	Name         string `json:"name"`
	SystemStatus monitoring.SystemStatus
}
