package monitoring

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	kube "github.com/gravitational/planet/Godeps/_workspace/src/k8s.io/kubernetes/pkg/client/unversioned"
	pb "github.com/gravitational/planet/lib/agent/proto/agentpb"
)

const systemNamespace = "kube-system"

// generic kubernetes healthz checker
func kubeHealthz(response io.Reader) error {
	payload, err := ioutil.ReadAll(response)
	if err != nil {
		return err
	}
	if !bytes.Equal(payload, []byte("ok")) {
		return fmt.Errorf("unexpected healthz response: %s", payload)
	}
	return nil
}

// KubeChecker is a Checker that communicates with the kube API server
type KubeChecker func(client *kube.Client) error

type kubeChecker struct {
	hostPort    string
	checkerFunc KubeChecker
}

func ConnectToKube(hostPort string) (*kube.Client, error) {
	var baseURL *url.URL
	var err error
	if hostPort == "" {
		return nil, trace.Errorf("hostPort cannot be empty")
	}
	baseURL, err = url.Parse(hostPort)
	if err != nil {
		return nil, err
	}
	config := &kube.Config{
		Host: baseURL.String(),
	}
	client, err := kube.New(config)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (r *kubeChecker) check(reporter reporter) {
	client, err := r.connect()
	if err != nil {
		reporter.add(err)
		return
	}
	err = r.checkerFunc(client)
	if err != nil {
		reporter.add(err)
	} else {
		reporter.addProbe(&pb.Probe{
			Status: pb.ServiceStatusType_ServiceRunning,
			Error:  "ok",
		})
	}
}

func (r *kubeChecker) connect() (*kube.Client, error) {
	return ConnectToKube(r.hostPort)
}

func etcdKubeServiceChecker(client *kube.Client) error {
	service, err := client.Services(systemNamespace).Get("etcd")
	if err != nil {
		return err
	}
	c := &http.Client{Timeout: defaultDialTimeout}
	addr, port := service.Spec.ClusterIP, service.Spec.Ports[0].Port
	var baseURL url.URL
	baseURL.Host = fmt.Sprintf("%s:%d", addr, port)
	baseURL.Scheme = "http"
	baseURL.Path = "/health"
	resp, err := c.Get(baseURL.String())
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	payload, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	healthy, err := etcdStatus(payload)
	if err != nil {
		return err
	}

	if !healthy {
		return fmt.Errorf("unexpected etcd (%s) response: %s", baseURL, payload)
	}
	return nil
}
