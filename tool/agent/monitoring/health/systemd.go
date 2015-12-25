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

// FIXME: factor out the name from Reporter.Add
func (r *systemChecker) Check(ctx *Context) {
	status, err := monitoring.Status()
	if err != nil {
		ctx.Reporter.Add("system", fmt.Sprintf("failed to check system health: %v", err))
		return
	}
	for _, service := range status.Services {
		ctx.Reporter.Add("system", fmt.Sprintf("service `%s` unhealthy: %s (%s)",
			service.Name, service.Status, service.Message))
	}
}
