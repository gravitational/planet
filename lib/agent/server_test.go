package agent

import (
	"testing"

	pb "github.com/gravitational/planet/lib/agent/proto/agentpb"
)

func TestSetsSystemStatusFromMemberStatuses(t *testing.T) {
	resp := &pb.StatusResponse{Status: &pb.SystemStatus{}}
	resp.Status.Members = []*pb.MemberStatus{
		{
			Name:   "foo",
			Status: pb.MemberStatus_Alive,
			Tags:   map[string]string{"role": "node"},
		},
		{
			Name:   "bar",
			Status: pb.MemberStatus_Failed,
			Tags:   map[string]string{"role": "master"},
		},
	}

	setSystemStatus(resp)
	if resp.Status.Status != pb.SystemStatus_Degraded {
		t.Errorf("expected system status %s but got %s", pb.SystemStatus_Degraded, resp.Status.Status)
	}
}

func TestSetsSystemStatusFromNodeStatuses(t *testing.T) {
	resp := &pb.StatusResponse{Status: &pb.SystemStatus{}}
	resp.Status.Nodes = []*pb.NodeStatus{
		{
			Name:   "foo",
			Status: pb.NodeStatus_Running,
		},
		{
			Name:   "bar",
			Status: pb.NodeStatus_Degraded,
			Probes: []*pb.Probe{
				{
					Checker: "qux",
					Status:  pb.Probe_Failed,
					Error:   "not available",
				},
			},
		},
	}

	// TODO: fail if no members?
	setSystemStatus(resp)
	if resp.Status.Status != pb.SystemStatus_Degraded {
		t.Errorf("expected system status %s but got %s", pb.SystemStatus_Degraded, resp.Status.Status)
	}
}

func TestDetectsNoMaster(t *testing.T) {
	resp := &pb.StatusResponse{Status: &pb.SystemStatus{}}
	resp.Status.Members = []*pb.MemberStatus{
		{
			Name:   "foo",
			Status: pb.MemberStatus_Alive,
			Tags:   map[string]string{"role": "node"},
		},
		{
			Name:   "bar",
			Status: pb.MemberStatus_Alive,
			Tags:   map[string]string{"role": "node"},
		},
	}

	setSystemStatus(resp)
	if resp.Status.Status != pb.SystemStatus_Degraded {
		t.Errorf("expected degraded system status but got %s", resp.Status)
	}
	if resp.Summary != errNoMaster.Error() {
		t.Errorf("expected '%s' but got '%s'", errNoMaster.Error(), resp.Summary)
	}
}

func TestSetsOkSystemStatus(t *testing.T) {
	resp := &pb.StatusResponse{Status: &pb.SystemStatus{}}
	resp.Status.Members = []*pb.MemberStatus{
		{
			Name:   "foo",
			Status: pb.MemberStatus_Alive,
			Tags:   map[string]string{"role": "node"},
		},
		{
			Name:   "bar",
			Status: pb.MemberStatus_Alive,
			Tags:   map[string]string{"role": "master"},
		},
	}
	resp.Status.Nodes = []*pb.NodeStatus{
		{
			Name:   "foo",
			Status: pb.NodeStatus_Running,
		},
		{
			Name:   "bar",
			Status: pb.NodeStatus_Running,
		},
	}

	expectedStatus := pb.SystemStatus_Running
	setSystemStatus(resp)
	if resp.Status.Status != expectedStatus {
		t.Errorf("expected system status %s but got %s", expectedStatus, resp.Status.Status)
	}
}
