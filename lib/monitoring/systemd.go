package monitoring

import (
	"bytes"
	"fmt"
	"os/exec"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/coreos/go-systemd/dbus"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
)

type systemd struct{}

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
	systemStatusCmd = []string{"/bin/systemctl", "is-system-running"}
)

func newSystemdService() Interface {
	return &systemd{}
}

func (r systemd) Status() ([]ServiceStatus, error) {
	conn, err := dbus.New()
	if err != nil {
		return nil, trace.Wrap(err, "failed to connect to dbus")
	}

	var units []dbus.UnitStatus
	units, err = conn.ListUnits()
	if err != nil {
		return nil, trace.Wrap(err, "failed to query systemd units")
	}

	var conditions []ServiceStatus
	for _, unit := range units {
		if unit.ActiveState == activeStateFailed || unit.LoadState == loadStateError {
			conditions = append(conditions, ServiceStatus{
				Name:    unit.Name,
				Status:  StatusFailed,
				Message: fmt.Sprintf("systemd: %s", unit.SubState),
			})
		}
	}

	return conditions, nil
}

func isSystemRunning() (string, error) {
	output, err := exec.Command(systemStatusCmd[0], systemStatusCmd[1:]...).CombinedOutput()
	if err != nil && !isExitError(err) {
		return SystemStatusUnknown, trace.Wrap(err)
	}

	var status string
	switch string(bytes.TrimSpace(output)) {
	case "initializing", "starting":
		status = SystemStatusLoading
	case "stopping", "offline":
		status = SystemStatusStopped
	case "degraded":
		status = SystemStatusDegraded
	case "running":
		status = SystemStatusRunning
	default:
		status = SystemStatusUnknown
	}
	return status, nil
}

func isExitError(err error) bool {
	if _, ok := err.(*exec.ExitError); ok {
		return true
	}
	return false
}
