// package health defines health checking primitives.
package health

import (
	"time"

	pb "github.com/gravitational/planet/lib/agent/proto/agentpb"
)

// Checker is an interface for executing a health check.
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
	// Add adds an health probe for a specific node.
	Add(probe *pb.Probe)
	// Status retrieves the collected status after executing all checks.
	Status() *pb.NodeStatus
}

// defaultReporter provides a default Reporter implementation.
type defaultReporter struct {
	status *pb.NodeStatus
}

func NewDefaultReporter(name string) Reporter {
	return &defaultReporter{status: &pb.NodeStatus{
		Name:   name,
		Status: pb.StatusType_SystemRunning,
	}}
}

func (r *defaultReporter) Add(probe *pb.Probe) {
	r.status.Probes = append(r.status.Probes, probe)
	if probe.Timestamp == nil {
		probe.Timestamp = pb.TimeToProto(time.Now())
	}
	if probe.Status == pb.ServiceStatusType_ServiceFailed {
		r.status.Status = pb.StatusType_SystemDegraded
	}
}

func (r *defaultReporter) Status() *pb.NodeStatus {
	return r.status
}
