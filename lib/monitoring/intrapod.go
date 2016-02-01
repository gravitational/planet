package monitoring

import (
	"fmt"
	"strings"
	"time"

	"github.com/coreos/fleet/log"
	"github.com/gravitational/trace"
	"k8s.io/kubernetes/pkg/api"
	kube "k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/fields"
	"k8s.io/kubernetes/pkg/labels"
	"k8s.io/kubernetes/pkg/util"
)

const testNamespace = "planettest"
const serviceName = "nettest"

func kubeIntrapodCommunication(client *kube.Client) error {

	svc, err := client.Services(testNamespace).Create(&api.Service{
		ObjectMeta: api.ObjectMeta{
			Name: serviceName,
			Labels: map[string]string{
				"name": serviceName,
			},
		},
		Spec: api.ServiceSpec{
			Ports: []api.ServicePort{{
				Protocol:   "TCP",
				Port:       8080,
				TargetPort: util.NewIntOrStringFromInt(8080),
			}},
			Selector: map[string]string{
				"name": serviceName,
			},
		},
	})
	if err != nil {
		return trace.Wrap(err, "failed to create test service named [%s] %v", svc.Name)
	}

	cleanupService := func() {
		if err = client.Services(testNamespace).Delete(svc.Name); err != nil {
			log.Infof("failed to delete svc %v: %v", svc.Name, err)
		}
	}
	defer cleanupService()

	nodes, err := client.Nodes().List(labels.Everything(), fields.Everything())
	if err != nil {
		return trace.Wrap(err, "failed to list nodes")
	}

	// filterNodes(nodes, func(node api.Node) bool {
	// 	return isNodeReadySetAsExpected(&node, true)
	// })

	if len(nodes.Items) < 2 {
		return trace.Errorf("expected at least 2 ready nodes, but got %d", len(nodes.Items))
	}

	podNames, err := launchNetTestPodPerNode(client, nodes, serviceName)
	if err != nil {
		return trace.Wrap(err, "failed start nettest pod")
	}

	cleanupPods := func() {
		for _, podName := range podNames {
			if err = client.Pods(testNamespace).Delete(podName, nil); err != nil {
				log.Infof("failed to delete pod %s: %v", podName, err)
			}
		}
	}
	defer cleanupPods()

	// By("waiting for the webserver pods to transition to Running state")
	for _, podName := range podNames {
		err = waitTimeoutForPodRunningInNamespace(client, podName, testNamespace, podStartTimeout)
		if err != nil {
			return trace.Wrap(err, "pod %s failed to transition to Running state", podName)
		}
	}

	// By("waiting for connectivity to be verified")
	passed := false

	var body []byte
	getDetails := func() ([]byte, error) {
		return client.Get().
			Namespace(testNamespace).
			Prefix("proxy").
			Resource("services").
			Name(svc.Name).
			Suffix("read").
			DoRaw()
	}

	getStatus := func() ([]byte, error) {
		return client.Get().
			Namespace(testNamespace).
			Prefix("proxy").
			Resource("services").
			Name(svc.Name).
			Suffix("status").
			DoRaw()
	}

	timeout := time.Now().Add(2 * time.Minute)
	for i := 0; !passed && timeout.After(time.Now()); i++ {
		time.Sleep(2 * time.Second)
		// log.Infof("making a proxy status call")
		// start := time.Now()
		body, err = getStatus()
		// log.Infof("proxy status call returned in %v", time.Since(start))
		if err != nil {
			// log.Infof("attempt %v: service/pod still starting: %v)", i, err)
			continue
		}
		// validate if the container was able to find peers
		switch {
		case string(body) == "pass":
			passed = true
		case string(body) == "running":
			// log.Infof("attempt %v: test still running", i)
		case string(body) == "fail":
			if body, err = getDetails(); err != nil {
				return trace.Wrap(err, "failed to read test details")
			} else {
				return trace.Wrap(err, "containers failed to find peers")
			}
		case strings.Contains(string(body), "no endpoints available"):
			// Logf("attempt %v: waiting on service/endpoints", i)
		default:
			return trace.Errorf("unexpected response: [%s]", body)
		}
	}
	return nil
}

// FIXME: original timeout is 5m due to serialized docker pulls
// Since we're pre-packaging test containers, this should not be an issue
// as we're always pulling from the local private registry.
const podStartTimeout = 5 * time.Second

// How often to poll pods and nodes.
const pollInterval = 2 * time.Second

type podCondition func(pod *api.Pod) (bool, error)

func waitTimeoutForPodRunningInNamespace(client *kube.Client, podName string, namespace string, timeout time.Duration) error {
	return waitForPodCondition(client, namespace, podName, "running", timeout, func(pod *api.Pod) (bool, error) {
		if pod.Status.Phase == api.PodRunning {
			// Logf("found pod '%s' on node '%s'", podName, pod.Spec.NodeName)
			return true, nil
		}
		if pod.Status.Phase == api.PodFailed {
			return true, trace.Errorf("pod in failed status: %s", fmt.Sprintf("%#v", pod))
		}
		return false, nil
	})
}

func waitForPodCondition(client *kube.Client, ns, podName, desc string, timeout time.Duration, condition podCondition) error {
	log.Infof("waiting up to %[1]v for pod %[2]s status to be %[3]s", timeout, podName, desc)
	for start := time.Now(); time.Since(start) < timeout; time.Sleep(pollInterval) {
		pod, err := client.Pods(ns).Get(podName)
		if err != nil {
			log.Infof("get pod %[1]s in namespace '%[2]s' failed, ignoring for %[3]v: %[4]v",
				podName, ns, pollInterval, err)
			continue
		}
		done, err := condition(pod)
		if done {
			// TODO: update to latest trace to wrap nil
			if err != nil {
				return trace.Wrap(err)
			}
			return nil
		}
		log.Infof("waiting for pod %[1]s in namespace '%[2]s' status to be '%[3]s'"+
			"(found phase: %[4]q, readiness: %[5]t) (%[6]v elapsed)",
			podName, ns, desc, pod.Status.Phase, podReady(pod), time.Since(start))
	}
	return trace.Errorf("gave up waiting for pod '%s' to be '%s' after %v", podName, desc, timeout)
}

func launchNetTestPodPerNode(client *kube.Client, nodes *api.NodeList, name string) ([]string, error) {
	podNames := []string{}
	totalPods := len(nodes.Items)

	for _, node := range nodes.Items {
		pod, err := client.Pods(testNamespace).Create(&api.Pod{
			ObjectMeta: api.ObjectMeta{
				GenerateName: name + "-",
				Labels: map[string]string{
					"name": name,
				},
			},
			Spec: api.PodSpec{
				Containers: []api.Container{
					{
						Name:  "webserver",
						Image: "nettest",
						Args: []string{
							"-service=" + name,
							//peers >= totalPods should be asserted by the container.
							//the nettest container finds peers by looking up list of svc endpoints.
							fmt.Sprintf("-peers=%d", totalPods),
							"-namespace=" + testNamespace},
						Ports: []api.ContainerPort{{ContainerPort: 8080}},
					},
				},
				NodeName:      node.Name,
				RestartPolicy: api.RestartPolicyNever,
			},
		})
		if err != nil {
			return nil, trace.Wrap(err, "failed to create pod")
		}
		log.Infof("created pod %s on node %s", pod.ObjectMeta.Name, node.Name)
		podNames = append(podNames, pod.ObjectMeta.Name)
	}
	return podNames, nil
}

// podReady returns whether pod has a condition of Ready with a status of true.
func podReady(pod *api.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == api.PodReady && cond.Status == api.ConditionTrue {
			return true
		}
	}
	return false
}
