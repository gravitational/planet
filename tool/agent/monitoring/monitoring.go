package monitoring

import (
	"github.com/gravitational/planet/lib/monitoring"
)

type StatusType string

const (
	StatusRunning  StatusType = "running"
	StatusDegraded            = "degraded"
)

type Status struct {
	Status StatusType   `json:"status"`
	Nodes  []NodeStatus `json:"nodes,omitempty"`
}

type NodeStatus struct {
	Name         string `json:"name"`
	SystemStatus monitoring.SystemStatus
}
