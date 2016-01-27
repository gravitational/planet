package health

import (
	"testing"

	pb "github.com/gravitational/planet/lib/agent/proto/agentpb"
)

func TestReporterSetsTimestamp(t *testing.T) {
	r := NewDefaultReporter("node")
	r.Add(&pb.Probe{
		Checker: "foo",
		Status:  pb.Probe_Failed,
		Error:   "not found",
	})
	numProbes := 1
	status := r.Status()
	if len(status.Probes) != numProbes {
		t.Fatalf("expected %d probe but got %d", numProbes, len(status.Probes))
	}
	probe := status.Probes[0]
	if probe.Timestamp == nil {
		t.Fatalf("expected probe timestamp to be non-null")
	}
}

func TestSetsNodeStatusFromProbes(t *testing.T) {
	r := NewDefaultReporter("node")
	for _, probe := range probes() {
		r.Add(probe)
	}
	expectedStatus := pb.NodeStatus_Degraded
	status := r.Status()
	if status.Status != expectedStatus {
		t.Fatalf("expected node status to be %s but got %s", expectedStatus, status.Status)
	}
}

func probes() []*pb.Probe {
	probes := []*pb.Probe{
		&pb.Probe{
			Checker: "foo",
			Status:  pb.Probe_Failed,
			Error:   "not found",
		},
		&pb.Probe{
			Checker: "bar",
			Status:  pb.Probe_Running,
		},
	}
	return probes
}
