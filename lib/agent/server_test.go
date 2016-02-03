package agent

import (
	"errors"
	"fmt"
	"net"
	"testing"

	log "github.com/Sirupsen/logrus"
	"github.com/gravitational/planet/lib/agent/health"
	pb "github.com/gravitational/planet/lib/agent/proto/agentpb"
	serf "github.com/hashicorp/serf/client"
	"golang.org/x/net/context"
)

func init() {
	if testing.Verbose() {
		log.Initialize("console", "INFO")
	}
}

func TestSetsSystemStatusFromMemberStatuses(t *testing.T) {
	resp := &pb.StatusResponse{Status: &pb.SystemStatus{}}
	resp.Status.Nodes = []*pb.NodeStatus{
		{
			MemberStatus: &pb.MemberStatus{
				Name:   "foo",
				Status: pb.MemberStatus_Alive,
				Tags:   map[string]string{"role": string(RoleNode)},
			},
		},
		{
			MemberStatus: &pb.MemberStatus{
				Name:   "bar",
				Status: pb.MemberStatus_Failed,
				Tags:   map[string]string{"role": string(RoleMaster)},
			},
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
			MemberStatus: &pb.MemberStatus{
				Name:   "foo",
				Status: pb.MemberStatus_Alive,
				Tags:   map[string]string{"role": string(RoleNode)},
			},
		},
		{
			Name:   "bar",
			Status: pb.NodeStatus_Degraded,
			MemberStatus: &pb.MemberStatus{
				Name:   "bar",
				Status: pb.MemberStatus_Alive,
				Tags:   map[string]string{"role": string(RoleMaster)},
			},
			Probes: []*pb.Probe{
				{
					Checker: "qux",
					Status:  pb.Probe_Failed,
					Error:   "not available",
				},
			},
		},
	}

	setSystemStatus(resp)
	if resp.Status.Status != pb.SystemStatus_Degraded {
		t.Errorf("expected system status %s but got %s", pb.SystemStatus_Degraded, resp.Status.Status)
	}
}

func TestDetectsNoMaster(t *testing.T) {
	resp := &pb.StatusResponse{Status: &pb.SystemStatus{}}
	resp.Status.Nodes = []*pb.NodeStatus{
		{
			MemberStatus: &pb.MemberStatus{
				Name:   "foo",
				Status: pb.MemberStatus_Alive,
				Tags:   map[string]string{"role": string(RoleNode)},
			},
		},
		{
			MemberStatus: &pb.MemberStatus{
				Name:   "bar",
				Status: pb.MemberStatus_Alive,
				Tags:   map[string]string{"role": string(RoleNode)},
			},
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
	resp.Status.Nodes = []*pb.NodeStatus{
		{
			Name:   "foo",
			Status: pb.NodeStatus_Running,
			MemberStatus: &pb.MemberStatus{
				Name:   "foo",
				Status: pb.MemberStatus_Alive,
				Tags:   map[string]string{"role": string(RoleNode)},
			},
		},
		{
			Name:   "bar",
			Status: pb.NodeStatus_Running,
			MemberStatus: &pb.MemberStatus{
				Name:   "bar",
				Status: pb.MemberStatus_Alive,
				Tags:   map[string]string{"role": string(RoleMaster)},
			},
		},
	}

	expectedStatus := pb.SystemStatus_Running
	setSystemStatus(resp)
	if resp.Status.Status != expectedStatus {
		t.Errorf("expected system status %s but got %s", expectedStatus, resp.Status.Status)
	}
}

func TestAgentProvidesStatus(t *testing.T) {
	for _, testCase := range agentTestCases {
		t.Logf("running test %s", testCase.comment)
		localNode := testCase.members[0].Name
		remoteNode := testCase.members[1].Name
		localServer := newLocalNode(localNode, remoteNode, testCase.rpcPort, testCase.members[:], testCase.checkers[0])
		remoteServer, err := newRemoteNode(remoteNode, localNode, testCase.rpcPort, testCase.members[:], testCase.checkers[1])
		if err != nil {
			t.Fatal(err)
		}
		req := &pb.StatusRequest{}
		resp, err := localServer.Status(context.TODO(), req)
		if err != nil {
			t.Error(err)
		}

		if resp.Status.Status != testCase.status {
			t.Errorf("expected status %s but got %s", testCase.status, resp.Status.Status)
		}
		remoteServer.Stop()
	}
}

var healthyTest = &fakeChecker{
	name: "healthy service",
}

var failedTest = &fakeChecker{
	name: "failing service",
	err:  errInvalidState,
}

var agentTestCases = []struct {
	comment  string
	status   pb.SystemStatus_Type
	members  [2]serf.Member
	checkers [][]health.Checker
	rpcPort  int
}{
	{
		comment: "Degraded due to a failed checker",
		status:  pb.SystemStatus_Degraded,
		members: [2]serf.Member{
			newMember("master", "alive"),
			newMember("node", "alive"),
		},
		checkers: [][]health.Checker{{healthyTest, failedTest}, {healthyTest, healthyTest}},
		rpcPort:  7676,
	},
	{
		comment: "Degraded due to a missing master node",
		status:  pb.SystemStatus_Degraded,
		members: [2]serf.Member{
			newMember("node-1", "alive"),
			newMember("node-2", "alive"),
		},
		checkers: [][]health.Checker{{healthyTest, healthyTest}, {healthyTest, healthyTest}},
		rpcPort:  7677,
	},
	{
		comment: "Running with all systems running",
		status:  pb.SystemStatus_Running,
		members: [2]serf.Member{
			newMember("master", "alive"),
			newMember("node", "alive"),
		},
		checkers: [][]health.Checker{{healthyTest, healthyTest}, {healthyTest, healthyTest}},
		rpcPort:  7678,
	},
}

func newLocalNode(node, peerNode string, rpcPort int, members []serf.Member, checkers []health.Checker) *server {
	agent := newAgent(node, peerNode, rpcPort, members, checkers)
	server := &server{agent: agent}
	return server
}

func newRemoteNode(node, peerNode string, rpcPort int, members []serf.Member, checkers []health.Checker) (*server, error) {
	network := "tcp"
	addr := fmt.Sprintf(":%d", rpcPort)
	listener, err := net.Listen(network, addr)
	if err != nil {
		return nil, err
	}

	agent := newAgent(node, peerNode, rpcPort, members, checkers)
	server := newRPCServer(agent, []net.Listener{listener})

	return server, nil
}

func newMember(name string, status string) serf.Member {
	result := serf.Member{
		Name:   name,
		Status: status,
		Tags:   map[string]string{"role": string(RoleNode)},
	}
	if name == "master" {
		result.Tags["role"] = string(RoleMaster)
	}
	return result
}

type fakeSerfClient struct {
	members []serf.Member
}

func (r *fakeSerfClient) Members() ([]serf.Member, error) {
	return r.members, nil
}

func (r *fakeSerfClient) Stream(filter string, eventc chan<- map[string]interface{}) (serf.StreamHandle, error) {
	return serf.StreamHandle(0), nil
}

func (r *fakeSerfClient) Stop(handle serf.StreamHandle) error {
	return nil
}

func (r *fakeSerfClient) Close() error {
	return nil
}

func (r *fakeSerfClient) Join(peers []string, replay bool) (int, error) {
	return 0, nil
}

type fakeCache struct {
	statuses []*pb.NodeStatus
}

func (r *fakeCache) UpdateNode(status *pb.NodeStatus) error {
	r.statuses = append(r.statuses, status)
	return nil
}

func (r fakeCache) RecentStatus(string) ([]*pb.Probe, error) {
	return nil, nil
}

func (r *fakeCache) Close() error {
	return nil
}

func testDialRPC(port int) dialRPC {
	return func(member *serf.Member) (*client, error) {
		addr := fmt.Sprintf(":%d", port)
		client, err := NewClient(addr)
		if err != nil {
			return nil, err
		}
		return client, err
	}
}

func newAgent(node, peerNode string, rpcPort int, members []serf.Member, checkers []health.Checker) *agent {
	return &agent{
		name:       node,
		serfClient: &fakeSerfClient{members: members},
		dialRPC:    testDialRPC(rpcPort),
		cache:      &fakeCache{},
		Checkers:   checkers,
	}
}

var errInvalidState = errors.New("invalid state")

type fakeChecker struct {
	err  error
	name string
}

func (r fakeChecker) Name() string { return r.name }

func (r *fakeChecker) Check(reporter health.Reporter) {
	if r.err != nil {
		reporter.Add(&pb.Probe{
			Checker: r.name,
			Error:   r.err.Error(),
			Status:  pb.Probe_Failed,
		})
	} else {
		reporter.Add(&pb.Probe{
			Checker: r.name,
			Status:  pb.Probe_Running,
		})
	}
}
