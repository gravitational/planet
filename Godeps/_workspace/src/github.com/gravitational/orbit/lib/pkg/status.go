package pkg

const (
	StatusRunning  = "running"
	StatusDegraded = "degraded"
	StatusStopped  = "stopped"
)

type Status struct {
	Status string      `json:"status"` // Status of the running container, one of 'running', 'stopped', 'degraded'
	Info   interface{} `json:"info"`   // App-specific information about the container
}
