package monitoring

type Status struct {
	Status StatusType   `json:"status"`
	Nodes  []NodeStatus `json:"nodes,omitempty"`
}

type StatusType string

const (
	StatusRunning  StatusType = "running"
	StatusDegraded            = "degraded"
)

type NodeStatus struct {
	Name   string          `json:"name"`
	Events []ServiceStatus `json:"events,omitempty"`
}

type ServiceStatus struct {
	Name   string            `json:"name"`
	Status ServiceStatusType `json:"status"`
	// Human-friendly description of the current service status
	Message string `json:"info"`
}

type ServiceStatusType string

const (
	ServiceStatusRunning ServiceStatusType = "running"
	ServiceStatusFailed                    = "failed"
)

// FIXME: move to systemd
type SystemStatusType string

const (
	SystemStatusRunning  SystemStatusType = "running"
	SystemStatusDegraded                  = "degraded"
	SystemStatusLoading                   = "loading"
	SystemStatusStopped                   = "stopped"
	SystemStatusUnknown                   = ""
)
