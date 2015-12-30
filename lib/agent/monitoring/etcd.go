package monitoring

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"time"
)

// defaultDialTimeout is the maximum amount of time a dial will wait for a connection to setup.
const defaultDialTimeout = 30 * time.Second

var etcdHealthzTags = Tags{
	"mode": {"master"},
}

func init() {
	AddChecker(newHTTPHealthzChecker("http://127.0.0.1:2379/health", etcdHealthzChecker), "etcd-healthz", etcdHealthzTags)
}

func etcdHealthzChecker(response io.Reader) error {
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
