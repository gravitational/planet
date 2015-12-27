package monitoring

import (
	"encoding/json"
	"fmt"
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

func init() {
	AddChecker(KubeChecker(checkEtcd), "etcd", etcdTags)
}

func checkEtcd(client *kube.Client) error {
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

	result := struct{ Health string }{}
	nresult := struct{ Health bool }{}
	payload, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	err = json.Unmarshal(payload, &result)
	if err != nil {
		err = json.Unmarshal(payload, &nresult)
	}
	if err != nil {
		return err
	}

	if result.Health != "true" && nresult.Health != true {
		return fmt.Errorf("unhealthy: %s", baseURL)
	}
	return nil
}
