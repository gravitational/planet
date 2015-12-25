// package check abstracts a process of running a health check.
package health

import (
	"fmt"
	"net/url"

	kube "github.com/gravitational/planet/Godeps/_workspace/src/k8s.io/kubernetes/pkg/client/unversioned"
)

// Context holds the context passed to Checker.Check.
type Context struct {
	Reporter
	*Config
}

type Config struct {
	KubeHostPort string
}

// Error is a result of a checker failing.
type Error struct {
	Name string
	Err  error
}

func (r *Error) Error() string {
	return fmt.Sprintf("%s: failed: %v", r.Name, r.Err)
}

// Checker defines an obligation to run a health check.
type Checker interface {
	// Check runs a health check and records any errors into the specified reporter.
	Check(*Context)
}

type Reporter interface {
	// Adds a problem report identified by a name with the specified payload.
	Add(name string, payload string)
}

type Tags map[string][]string

// Tester describes an instance of a health checker.
type Tester struct {
	Name    string
	Tags    Tags
	Checker Checker
}

var Testers []Tester

func AddChecker(checker Checker, name string, tags Tags) {
	Testers = append(Testers, Tester{Checker: checker, Name: name, Tags: tags})
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

func (r KubeChecker) Check(ctx *Context) {
	client, err := connectToKube(ctx.KubeHostPort)
	if err != nil {
		// FIXME: identify the checker func by name
		ctx.Reporter.Add("kube-checker", err.Error())
		return
	}
	err = r(client)
	if err != nil {
		ctx.Reporter.Add("kube-checker", err.Error())
	}
}
