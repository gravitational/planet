package monitoring

type Status struct {
	SystemStatus SystemStatusType `json:"status"`
	Nodes        []NodeStatus     `json:"nodes,omitempty"`
}

type SystemStatusType string

const (
	SystemStatusRunning  SystemStatusType = "running"
	SystemStatusDegraded                  = "degraded"
)

type NodeStatus struct {
	Name   string  `json:"name"`
	Events []Event `json:"events,omitempty"`
}

type Event struct {
	Name    string     `json:"name"`
	Service string     `json:"service"`
	Status  StatusType `json:"status"`
	// Human-friendly description of the current service status
	Message string `json:"info"`
}

type StatusType string

const (
	StatusRunning StatusType = "running"
	StatusFailed             = "failed"
)
