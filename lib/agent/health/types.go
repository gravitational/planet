package health

import "time"

// Status describes the health status of the cluster.
type Status struct {
	SystemStatus SystemStatusType `json:"status"`
	// Nodes lists statuses of individual nodes.
	Nodes []NodeStatus `json:"nodes,omitempty"`
}

type SystemStatusType string

const (
	SystemStatusRunning  SystemStatusType = "running"
	SystemStatusDegraded                  = "degraded"
)

// NodeStatus represents a result of a health check for a single node.
type NodeStatus struct {
	Name string `json:"name"`
	// Probes lists all the health probes collected during the health check.
	// For simplicity, only probes for alerts are collected.  Thus, for a healthy
	// system, the Probes will be empty.
	Probes []Probe `json:"events,omitempty"`
}

// NodeStats represents a specific node stat point.
type NodeStats struct {
	Timestamp time.Time `json:"timestamp"`

	Probes []Probe
}

// Probe represents a health probe.
type Probe struct {
	// Checker names the checker that generated the probe.
	Checker string `json:"checker"`
	// Service is an auxiliary field that can be used to provide more details for the
	// checker activity.
	Service string `json:"service"`
	// Status defines the status of this probe.
	Status StatusType `json:"status"`
	// Human-friendly description of the current status.
	Message string `json:"info"`
}

type StatusType string

const (
	StatusRunning StatusType = "running"
	StatusFailed             = "failed"
)
