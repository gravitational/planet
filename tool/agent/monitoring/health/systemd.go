package health

import (
	"fmt"

	"github.com/gravitational/planet/lib/monitoring"
)

// systemChecker is a health checker for services managed by systemd/monit.
type systemChecker struct{}

var systemTags = Tags{
	"mode": {"master", "node"},
}

func init() {
	AddChecker(&systemChecker{}, "system", csTags)
}

func (r *systemChecker) Check(reporter Reporter, config *Config) {
	status, err := monitoring.Status()
	if err != nil {
		reporter.Add(fmt.Errorf("failed to check system health: %v", err))
		return
	}
	for _, service := range status.Services {
		reporter.Add(fmt.Errorf("service `%s` unhealthy: %s (%s)",
			service.Name, service.Status, service.Message))
	}
}
