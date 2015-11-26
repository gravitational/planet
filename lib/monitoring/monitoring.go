package monitoring

import "github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"

type (
	Monitor interface {
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
	State_running State = "running"
	State_failed        = "failed"
)

type SystemState string

const (
	SystemState_running  SystemState = "running"
	SystemState_degraded             = "degraded"
	SystemState_loading              = "loading"
	SystemState_stopped              = "stopped"
	SystemState_unknown              = ""
)

func Status() (*SystemStatus, error) {
	var (
		monit             Monitor
		systemd           Monitor
		monitConditions   []ServiceStatus
		systemdConditions []ServiceStatus
		conditions        []ServiceStatus
		err               error
		systemState       SystemState
	)

	monit, err = newMonitService()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	monitConditions, err = monit.Status()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	systemd, err = newSystemdService()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	systemdConditions, err = systemd.Status()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	systemState, err = isSystemRunning()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	conditions = append([]ServiceStatus{}, monitConditions...)
	conditions = append(conditions, systemdConditions...)

	return &SystemStatus{
		SystemState: systemState,
		Services:    conditions,
	}, nil
}
