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

// Checkers is a collection of checkers.
// It implements CheckerRepository interface.
type Checkers []Checker

func (r *Checkers) AddChecker(checker Checker) {
	*r = append(*r, checker)
}

// CheckerRepository represents a collection of checkers.
type CheckerRepository interface {
	AddChecker(checker Checker)
}

// Reporter defines an obligation to report structured errors.
type Reporter interface {
	// Add adds an health probe for a specific node.
	Add(probe *pb.Probe)
	// Status retrieves the collected status after executing all checks.
	GetProbes() []*pb.Probe
}

// Probes is a list of probes.
// It implements the Reporter interface.
type Probes []*pb.Probe

func (r *Probes) Add(probe *pb.Probe) {
	*r = append(*r, probe)
	if probe.Timestamp == nil {
		probe.Timestamp = pb.NewTimeToProto(time.Now())
	}
}

func (r Probes) GetProbes() []*pb.Probe {
	return []*pb.Probe(r)
}

// Status computes the node status based on collected probes.
func (r Probes) Status() pb.NodeStatus_Type {
	result := pb.NodeStatus_Running
	for _, probe := range r {
		if probe.Status == pb.Probe_Failed {
			result = pb.NodeStatus_Degraded
			break
		}
	}
	return result
}
