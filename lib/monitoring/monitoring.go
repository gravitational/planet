package monitoring

import (
	"errors"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
)

type (
	Interface interface {
		Status() ([]ServiceStatus, error)
	}

	SystemStatus struct {
		Status   string          `json:"status"`
		Services []ServiceStatus `json:"services,omitempty"`
	}

	ServiceStatus struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		// Human-friendly description of the current service status
		Message string `json:"info"`
	}
)

const (
	StatusRunning = "running"
	StatusFailed  = "failed"
)

const (
	SystemStatusRunning  = "running"
	SystemStatusDegraded = "degraded"
	SystemStatusLoading  = "loading"
	SystemStatusStopped  = "stopped"
	SystemStatusUnknown  = ""
)

var ErrMonitorNotReady = errors.New("monitor service not ready")

func Status() (*SystemStatus, error) {
	systemStatus, err := isSystemRunning()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	systemdConditions, err := newSystemdService().Status()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	monitConditions, err := newMonitService().Status()
	if err != nil && err != ErrMonitorNotReady {
		return nil, trace.Wrap(err)
	}

	conditions := append([]ServiceStatus{}, systemdConditions...)
	conditions = append(conditions, monitConditions...)

	if len(conditions) > 0 && systemStatus == SystemStatusRunning {
		systemStatus = SystemStatusDegraded
	}

	return &SystemStatus{
		Status:   systemStatus,
		Services: conditions,
	}, nil
}
