// package health defines health checking primitives.
package health

import (
	pb "github.com/gravitational/planet/lib/agent/proto/agentpb"
)

// Checker defines an obligation to run a health check.
type Checker interface {
	Name() string
	// Check runs a health check and records any errors into the specified reporter.
	Check(Reporter)
}

type Checkers []Checker

func (r *Checkers) AddChecker(checker Checker) {
	*r = append(*r, checker)
}

type CheckerRepository interface {
	AddChecker(checker Checker)
}

// Reporter defines an obligation to report structured errors.
type Reporter interface {
	// Add adds an error report for the checker named name
	Add(checker string, err error)
	AddProbe(probe *pb.Probe)
	Status() *pb.NodeStatus
}

// defaultReporter provides a default Reporter implementation.
type defaultReporter struct {
	status *pb.NodeStatus
}

func NewDefaultReporter(name string) Reporter {
	return &defaultReporter{status: &pb.NodeStatus{Name: name}}
}

func (r *defaultReporter) Add(checker string, err error) {
	r.status.Probes = append(r.status.Probes, &pb.Probe{
		Checker: checker,
		Error:   err.Error(),
		Status:  pb.ServiceStatusType_ServiceFailed,
	})
}

func (r *defaultReporter) AddProbe(probe *pb.Probe) {
	r.status.Probes = append(r.status.Probes, probe)
}

func (r *defaultReporter) Status() *pb.NodeStatus {
	return r.status
}
