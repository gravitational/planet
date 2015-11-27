package monitoring

import (
	"errors"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
)

type (
	Interface interface {
		Status() ([]ServiceStatus, error)
	}

	ServiceStatus struct {
		Name string `json:"name"`
		// Timestamp time.Time
		State State `json:"status"`
		// Human-friendly description of the current service state
		Message string `json:"info"`
	}

	SystemStatus struct {
		SystemState `json:"state"`
		Services    []ServiceStatus `json:"services"`
	}
)

type State string

const (
	StateRunning State = "running"
	StateFailed        = "failed"
)

type SystemState string

const (
	SystemStateRunning  SystemState = "running"
	SystemStateDegraded             = "degraded"
	SystemStateLoading              = "loading"
	SystemStateStopped              = "stopped"
	SystemStateUnknown              = ""
)

var ErrMonitorNotReady = errors.New("monitor service not ready")

func Status() (*SystemStatus, error) {
	monitConditions, err := newMonitService().Status()
	if err != nil && err != ErrMonitorNotReady {
		return nil, trace.Wrap(err)
	}

	systemdConditions, err := newSystemdService().Status()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	systemState, err := isSystemRunning()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	conditions := append([]ServiceStatus{}, monitConditions...)
	conditions = append(conditions, systemdConditions...)

	if len(conditions) > 0 && systemState == SystemStateRunning {
		systemState = SystemStateDegraded
	}

	return &SystemStatus{
		SystemState: systemState,
		Services:    conditions,
	}, nil
}
