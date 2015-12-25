package health

import (
	"fmt"

	"github.com/gravitational/planet/Godeps/_workspace/src/k8s.io/kubernetes/pkg/api"
	"github.com/gravitational/planet/Godeps/_workspace/src/k8s.io/kubernetes/pkg/fields"
	"github.com/gravitational/planet/Godeps/_workspace/src/k8s.io/kubernetes/pkg/labels"
	// kube "github.com/gravitational/planet/Godeps/_workspace/src/k8s.io/kubernetes/pkg/client/unversioned"
)

// TODO: dns checkers.
var csTags = map[string]string{
	"mode": "master",
}

type componentStatusChecker struct{}

func init() {
	AddChecker(&componentStatusChecker{}, "cs", csTags)
}

func (r *componentStatusChecker) Check(ctx *Context) {
	client, err := connectToKube(ctx.KubeHostPort)
	if err != nil {
		ctx.Reporter.Add("cs", fmt.Sprintf("failed to connect to kube: %v", err))
		return
	}
	statuses, err := client.ComponentStatuses().List(labels.Everything(), fields.Everything())
	if err != nil {
		ctx.Reporter.Add("cs", fmt.Sprintf("failed to query component statuses: %v", err))
		return
	}
	for _, item := range statuses.Items {
		for _, condition := range item.Conditions {
			if condition.Type != api.ComponentHealthy || condition.Status != api.ConditionTrue {
				ctx.Reporter.Add("cs", fmt.Sprintf("component unhealthy (%s): %s (%s)",
					condition.Type, condition.Message, condition.Error))
			}
		}
	}
}
