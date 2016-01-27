package health

import (
	"testing"

	pb "github.com/gravitational/planet/lib/agent/proto/agentpb"
)

func TestReporterSetsTimestamp(t *testing.T) {
	var r Probes
	r.Add(&pb.Probe{
		Checker: "foo",
		Status:  pb.Probe_Failed,
		Error:   "not found",
	})
	numProbes := 1
	if len(r) != numProbes {
		t.Fatalf("expected %d probe but got %d", numProbes, len(r))
	}
	probe := r[0]
	if probe.Timestamp == nil {
		t.Fatalf("expected probe timestamp to be non-null")
	}
}

func TestSetsNodeStatusFromProbes(t *testing.T) {
	var r Probes
	for _, probe := range probes() {
		r.Add(probe)
	}
	expectedStatus := pb.NodeStatus_Degraded
	status := r.Status()
	if status != expectedStatus {
		t.Fatalf("expected node status to be %s but got %s", expectedStatus, status)
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
