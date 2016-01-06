package monitoring

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"

	kube "github.com/gravitational/planet/Godeps/_workspace/src/k8s.io/kubernetes/pkg/client/unversioned"
)

// generic kubernetes healthz checker
func kubeHealthz(response io.Reader) error {
	payload, err := ioutil.ReadAll(response)
	if err != nil {
		return err
	}
	if !bytes.Equal(payload, []byte("ok")) {
		return errHealthzCheck
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
		hostPort = "127.0.0.1:8080"
		baseURL = &url.URL{
			Host:   hostPort,
			Scheme: "http",
		}
	} else {
		baseURL, err = url.Parse(hostPort)
		if err != nil {
			return nil, err
		}
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
	}
}

func (r *kubeChecker) connect() (*kube.Client, error) {
	return ConnectToKube(r.hostPort)
}

func etcdKubeServiceChecker(client *kube.Client) error {
	const namespace = "kube-system"
	service, err := client.Services(namespace).Get("etcd")
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
		return fmt.Errorf("etcd at %s unhealthy", baseURL)
	}
	return nil
}
