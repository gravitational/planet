package monitoring

import (
	"fmt"

	"github.com/gravitational/planet/Godeps/_workspace/src/k8s.io/kubernetes/pkg/api"
	"github.com/gravitational/planet/Godeps/_workspace/src/k8s.io/kubernetes/pkg/fields"
	"github.com/gravitational/planet/Godeps/_workspace/src/k8s.io/kubernetes/pkg/labels"
)

var csTags = Tags{
	"mode": {"master"},
}

type componentStatusChecker struct {
	hostPort string
}

func (r *componentStatusChecker) check(reporter reporter) {
	client, err := connectToKube(r.hostPort)
	if err != nil {
		reporter.add(fmt.Errorf("failed to connect to kube: %v", err))
		return
	}
	statuses, err := client.ComponentStatuses().List(labels.Everything(), fields.Everything())
	if err != nil {
		reporter.add(fmt.Errorf("failed to query component statuses: %v", err))
		return
	}
	for _, item := range statuses.Items {
		for _, condition := range item.Conditions {
			if condition.Type != api.ComponentHealthy || condition.Status != api.ConditionTrue {
				reporter.addEvent(Event{
					Service: item.Name,
					Status:  StatusFailed,
					Message: fmt.Sprintf("%s (%s)", condition.Message, condition.Error),
				})
			}
		}
	}
}
