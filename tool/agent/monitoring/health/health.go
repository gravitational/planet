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

type checker interface {
	// Check runs a health check and records any errors into the specified reporter.
	check(reporter, *Config)
}

// reporter defines an obligation to report errors.
type reporter interface {
	add(err error)
}

// Reporter defines an obligation to report errors with a specified name.
type Reporter interface {
	Add(name string, err error)
}

type Tags map[string][]string

// Tester describes an instance of a health checker.
type Tester struct {
	checker
	Tags Tags
	Name string
}

// List of registered testers.
var Testers []Tester

// AddChecker registers a new checker specified by name and a set of tags.
//
// Tags can be used to annotate a checker with a set of labels.
// For instance, checkers can easily be bound to a certain agent (and thus,
// a certain node) by starting an agent with the same set of tags as those
// specified by the checker and the checker will only run on that agent.
func AddChecker(checker checker, name string, tags Tags) {
	Testers = append(Testers, Tester{checker: checker, Name: name, Tags: tags})
}

// delegatingReporter is a bridge between internal reporter and exported Reporter
// implementations.
// It implements reporter and delegates to the given Reporter using the specified
// tester.
type delegatingReporter struct {
	Reporter
	tester *Tester
}

func (r *Tester) Check(reporter Reporter, config *Config) {
	rep := &delegatingReporter{Reporter: reporter, tester: r}
	r.check(rep, config)
}

func (r *delegatingReporter) add(err error) {
	r.Reporter.Add(r.tester.Name, err)
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

func (r KubeChecker) check(reporter reporter, config *Config) {
	client, err := connectToKube(config.KubeHostPort)
	if err != nil {
		reporter.add(err)
		return
	}
	err = r(client)
	if err != nil {
		reporter.add(err)
	}
}
