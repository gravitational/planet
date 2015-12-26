package health

import (
	"fmt"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/k8s.io/kubernetes/pkg/api"
	"github.com/gravitational/planet/Godeps/_workspace/src/k8s.io/kubernetes/pkg/fields"
	"github.com/gravitational/planet/Godeps/_workspace/src/k8s.io/kubernetes/pkg/labels"
)

// TODO: dns checkers.
var csTags = Tags{
	"mode": {"master"},
}

type componentStatusChecker struct{}

func init() {
	AddChecker(&componentStatusChecker{}, "cs", csTags)
}

func (r *componentStatusChecker) check(reporter reporter, config *Config) {
	client, err := connectToKube(config.KubeHostPort)
	if err != nil {
		reporter.add(fmt.Errorf("failed to connect to kube: %v", err))
		return
	}
	statuses, err := client.ComponentStatuses().List(labels.Everything(), fields.Everything())
	if err != nil {
		reporter.add(fmt.Errorf("failed to query component statuses: %v", err))
		return
	}
	log.Infof("componentstatuses: %#v", statuses)
	for _, item := range statuses.Items {
		for _, condition := range item.Conditions {
			if condition.Type != api.ComponentHealthy || condition.Status != api.ConditionTrue {
				reporter.add(fmt.Errorf("component unhealthy (%s): %s (%s)",
					condition.Type, condition.Message, condition.Error))
			}
		}
	}
}
