package monitoring

import (
	"fmt"
)

// systemChecker is a health checker for services managed by systemd/monit.
type systemChecker struct{}

type serviceStatus struct {
	name   string
	status StatusType
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

var systemTags = Tags{
	"mode": {"master", "node"},
}

func init() {
	addChecker(&systemChecker{}, "system", systemTags)
}

func (r *systemChecker) check(reporter reporter, config *Config) {
	systemStatus, err := isSystemRunning()
	if err != nil {
		reporter.add(fmt.Errorf("failed to check system health: %v", err))
	}

	systemdConditions, err := systemdStatus()
	if err != nil {
		reporter.add(fmt.Errorf("failed to check systemd status: %v", err))
	}

	monitConditions, err := monitStatus()
	if err != nil && err != errMonitorNotReady {
		reporter.add(fmt.Errorf("failed to check monit status: %v", err))
	}

	conditions := append([]serviceStatus{}, systemdConditions...)
	conditions = append(conditions, monitConditions...)

	if len(conditions) > 0 && systemStatusType(systemStatus) == systemStatusRunning {
		systemStatus = systemStatusDegraded
	}

	// FIXME: do away with system state
	// if systemStatus != systemStatusRunning {
	// 	reporter.add(fmt.Errorf("system status: %v", systemStatus))
	// }

	for _, condition := range conditions {
		reporter.addEvent(Event{
			Service: condition.name,
			Status:  condition.status,
			Message: condition.err.Error(),
		})
	}
}
