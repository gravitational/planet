package monitoring

import (
	"bytes"
	"fmt"
	"os/exec"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/coreos/go-systemd/dbus"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	pb "github.com/gravitational/planet/lib/agent/proto/agentpb"
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

var systemStatusCmd = []string{"/bin/systemctl", "is-system-running"}

// systemChecker is a health checker for services managed by systemd/monit.
type systemdChecker struct{}

type serviceStatus struct {
	name   string
	status pb.ServiceStatusType
	err    error
}

type systemStatusType string

const (
	systemStatusRunning  systemStatusType = "running"
	systemStatusDegraded                  = "degraded"
	systemStatusLoading                   = "loading"
	systemStatusStopped                   = "stopped"
	systemStatusUnknown                   = ""
)

func (r systemdChecker) check(reporter reporter) {
	systemStatus, err := isSystemRunning()
	if err != nil {
		reporter.add(fmt.Errorf("failed to check system health: %v", err))
	}

	conditions, err := systemdStatus()
	if err != nil {
		reporter.add(fmt.Errorf("failed to check systemd status: %v", err))
	}

	if len(conditions) > 0 && systemStatusType(systemStatus) == systemStatusRunning {
		systemStatus = systemStatusDegraded
	}

	// FIXME: do away with system state
	// if systemStatus != systemStatusRunning {
	// 	reporter.add(fmt.Errorf("system status: %v", systemStatus))
	// }

	for _, condition := range conditions {
		reporter.addProbe(&pb.Probe{
			Extra:  condition.name,
			Status: condition.status,
			Error:  condition.err.Error(),
		})
	}
}

func systemdStatus() ([]serviceStatus, error) {
	conn, err := dbus.New()
	if err != nil {
		return nil, trace.Wrap(err, "failed to connect to dbus")
	}

	var units []dbus.UnitStatus
	units, err = conn.ListUnits()
	if err != nil {
		return nil, trace.Wrap(err, "failed to query systemd units")
	}

	var conditions []serviceStatus
	for _, unit := range units {
		if unit.ActiveState == activeStateFailed || unit.LoadState == loadStateError {
			conditions = append(conditions, serviceStatus{
				name:   unit.Name,
				status: pb.ServiceStatusType_ServiceFailed,
				err:    fmt.Errorf("%s", unit.SubState),
			})
		}
	}

	return conditions, nil
}

func isSystemRunning() (systemStatusType, error) {
	output, err := exec.Command(systemStatusCmd[0], systemStatusCmd[1:]...).CombinedOutput()
	if err != nil && !isExitError(err) {
		return systemStatusUnknown, trace.Wrap(err)
	}

	var status systemStatusType
	switch string(bytes.TrimSpace(output)) {
	case "initializing", "starting":
		status = systemStatusLoading
	case "stopping", "offline":
		status = systemStatusStopped
	case "degraded":
		status = systemStatusDegraded
	case "running":
		status = systemStatusRunning
	default:
		status = systemStatusUnknown
	}
	return status, nil
}

func isExitError(err error) bool {
	if _, ok := err.(*exec.ExitError); ok {
		return true
	}
	return false
}
