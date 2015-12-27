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

var systemTags = Tags{
	"mode": {"master", "node"},
}

func init() {
	AddChecker(&systemChecker{}, "system", csTags)
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

	if len(conditions) > 0 && SystemStatusType(systemStatus) == SystemStatusRunning {
		systemStatus = SystemStatusDegraded
	}

	if systemStatus != SystemStatusRunning {
		reporter.add(fmt.Errorf("system status: %v", systemStatus))
	}

	for _, condition := range conditions {
		reporter.add(fmt.Errorf("service `%s` status: %s (%v)",
			condition.name, condition.status, condition.err))
	}
}
