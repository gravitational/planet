package monitoring

import (
	"bytes"
	"io"
	"io/ioutil"
)

var kubeApiServerTags = Tags{
	"mode": {"master"},
}

var kubeletTags = Tags{
	"mode": {"node"},
}

func init() {
	addChecker(newHTTPHealthzChecker("http://127.0.0.1:8080/healthz", kubeHealthz), "kube-apiserver", kubeApiServerTags)
	addChecker(newHTTPHealthzChecker("http://127.0.0.1:10248/healthz", kubeHealthz), "kubelet", kubeletTags)
}

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
