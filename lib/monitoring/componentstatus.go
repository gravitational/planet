package monitoring

import (
	"fmt"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	"github.com/gravitational/planet/Godeps/_workspace/src/k8s.io/kubernetes/pkg/api"
	"github.com/gravitational/planet/Godeps/_workspace/src/k8s.io/kubernetes/pkg/fields"
	"github.com/gravitational/planet/Godeps/_workspace/src/k8s.io/kubernetes/pkg/labels"
	pb "github.com/gravitational/planet/lib/agent/proto/agentpb"
)

// componentStatusChecker tests and reports health failures in kubernetes
// components (controller-manager, scheduler, etc.)
type componentStatusChecker struct {
	hostPort string
}

func (r *componentStatusChecker) check(reporter reporter) {
	client, err := ConnectToKube(r.hostPort)
	if err != nil {
		reporter.add(trace.Errorf("failed to connect to kube: %v", err))
		return
	}
	statuses, err := client.ComponentStatuses().List(labels.Everything(), fields.Everything())
	if err != nil {
		reporter.add(trace.Errorf("failed to query component statuses: %v", err))
		return
	}
	for _, item := range statuses.Items {
		for _, condition := range item.Conditions {
			if condition.Type != api.ComponentHealthy || condition.Status != api.ConditionTrue {
				reporter.addProbe(&pb.Probe{
					Extra:  item.Name,
					Status: pb.Probe_Failed,
					Error:  fmt.Sprintf("%s (%s)", condition.Message, condition.Error),
				})
			} else {
				reporter.addProbe(&pb.Probe{
					Extra:  item.Name,
					Status: pb.Probe_Running,
				})
			}
		}
	}
}
