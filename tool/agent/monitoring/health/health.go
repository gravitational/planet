// package check abstracts a process of running health checks.
package health

import (
	"net/url"

	kube "github.com/gravitational/planet/Godeps/_workspace/src/k8s.io/kubernetes/pkg/client/unversioned"
)

type Config struct {
	KubeHostPort string
}

// Checker defines an obligation to run a health check.
type Checker interface {
	// Check runs a health check and records any errors into the specified reporter.
	Check(Reporter, *Config)
}

type Reporter interface {
	// Add adds a new error.
	Add(err error)
}

type NamedReporter interface {
	// AddNamed adds a new problem payload with the specified name.
	AddNamed(name string, err error)
}

type Tags map[string][]string

// Tester describes an instance of a health checker.
type Tester struct {
	Checker
	Tags Tags
	Name string
}

var Testers []Tester

func AddChecker(checker Checker, name string, tags Tags) {
	Testers = append(Testers, Tester{Checker: checker, Name: name, Tags: tags})
}

type delegatingReporter struct {
	NamedReporter
	tester *Tester
}

func (r *Tester) Check(reporter Reporter, config *Config) {
	if nreporter, ok := reporter.(NamedReporter); ok {
		rep := &delegatingReporter{NamedReporter: nreporter, tester: r}
		r.Checker.Check(rep, config)
	} else {
		r.Checker.Check(reporter, config)
	}
}

func (r *delegatingReporter) Add(err error) {
	r.NamedReporter.AddNamed(r.tester.Name, err)
}

// KubeChecker is a Checker that needs to communicate with a kube API server
type KubeChecker func(client *kube.Client) error

func connectToKube(host string) (*kube.Client, error) {
	var baseURL *url.URL
	var err error
	if host == "" {
		host = "127.0.0.1:8080"
		baseURL = &url.URL{
			Host:   host,
			Scheme: "http",
		}
	} else {
		baseURL, err = url.Parse(host)
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

func (r KubeChecker) Check(reporter Reporter, config *Config) {
	client, err := connectToKube(config.KubeHostPort)
	if err != nil {
		reporter.Add(err)
		return
	}
	err = r(client)
	if err != nil {
		reporter.Add(err)
	}
}
