package monitoring

import (
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
)

type (
	// almost verbatim copy (github.com/coreos/go-systemd/dbus/methods.go#UnitStatus)
	unitStatus struct {
		Name        string // The primary unit name as string
		Description string // The human readable description string
		LoadState   string // The load state (i.e. whether the unit file has been loaded successfully)
		ActiveState string // The active state (i.e. whether the unit is currently started or not)
		SubState    string // The sub state (a more fine-grained version of the active state that is specific to the unit type, which the active state is not)
		Followed    string // A unit that is being followed in its state by this unit, if there is any, otherwise the empty string.
		Path        string // The unit object path
		JobId       uint32 // If there is a job queued for the job unit the numeric job id, 0 otherwise
		JobType     string // The job type as string
		JobPath     string // The job object path
	}

	systemd struct{}
)

type loadState string

const (
	loadStateLoaded   loadState = "loaded"
	loadStateError              = "error"
	loadStateMasked             = "masked"
	loadStateNotFound           = "not-found"
)

type activeState string

const (
	activeStateActive       activeState = "active"
	activeStateReloading                = "reloading"
	activeStateInactive                 = "inactive"
	activeStateFailed                   = "failed"
	activeStateActivating               = "activating"
	activeStateDeactivating             = "deactivating"
)

var (
	queryCmd       = "/usr/bin/systemd-query"
	systemStateCmd = []string{"/usr/bin/systemctl", "is-system-running"}
)

func newSystemdService() (Monitor, error) {
	return &systemd{}, nil
}

func (r systemd) Status() ([]ServiceStatus, error) {
	var (
		data       []byte
		err        error
		units      []unitStatus
		conditions []ServiceStatus
	)

	data, err = exec.Command(queryCmd).CombinedOutput()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	if err = json.Unmarshal(data, &units); err != nil {
		return nil, trace.Wrap(err)
	}

	for _, unit := range units {
		if unit.ActiveState == activeStateFailed || unit.LoadState == loadStateError {
			conditions = append(conditions, ServiceStatus{
				Name:    unit.Name,
				State:   StateFailed,
				Message: fmt.Sprintf("systemd: %s", unit.SubState),
			})
		}
	}
	return conditions, nil
}

func isSystemRunning() (state SystemState, err error) {
	var output []byte

	output, err = exec.Command(systemStateCmd[0], systemStateCmd[1:]...).CombinedOutput()
	if err != nil {
		return SystemStateUnknown, trace.Wrap(err)
	}

	state = SystemStateUnknown
	switch string(output) {
	case "initializing", "starting":
		state = SystemStateLoading
	case "stopping", "offline":
		state = SystemStateStopped
	case "degraded":
		state = SystemStateDegraded
	case "running":
		state = SystemStateRunning
	}
	return state, nil
}
