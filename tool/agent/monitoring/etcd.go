package monitoring

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	kube "github.com/gravitational/planet/Godeps/_workspace/src/k8s.io/kubernetes/pkg/client/unversioned"
)

// defaultDialTimeout is the maximum amount of time a dial will wait for a connection to setup.
const defaultDialTimeout = 30 * time.Second

var etcdTags = Tags{
	"mode": {"node"},
}

var etcdHealthzTags = Tags{
	"mode": {"master"},
}

func init() {
	addChecker(KubeChecker(etcdKubeServiceCheck), "etcd", etcdTags)
	addChecker(newHTTPHealthzChecker("http://127.0.0.1:2379/health", etcdHealthz), "etcd-healthz", etcdHealthzTags)
}

func etcdKubeServiceCheck(client *kube.Client) error {
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

func etcdHealthz(response io.Reader) error {
	payload, err := ioutil.ReadAll(response)
	if err != nil {
		return err
	}

	healthy, err := etcdStatus(payload)
	if err != nil {
		return err
	}

	if !healthy {
		return errHealthzCheck
	}
	return nil
}

func etcdStatus(payload []byte) (healthy bool, err error) {
	result := struct{ Health string }{}
	nresult := struct{ Health bool }{}
	err = json.Unmarshal(payload, &result)
	if err != nil {
		err = json.Unmarshal(payload, &nresult)
	}
	if err != nil {
		return false, err
	}

	return (result.Health == "true" || nresult.Health == true), nil
}
