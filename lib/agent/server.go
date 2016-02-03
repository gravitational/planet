package agent

import (
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/gravitational/log"
	pb "github.com/gravitational/planet/lib/agent/proto/agentpb"
	"github.com/gravitational/trace"
	serf "github.com/hashicorp/serf/client"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

// Default RPC port.
const RPCPort = 7575 // FIXME: use serf to discover agents

var errNoMaster = errors.New("master node unavailable")

// server implements RPC for an agent.
type server struct {
	*grpc.Server
	agent *agent
}

// Status reports the health status of a serf cluster by iterating over the list
// of currently active cluster members and collecting their respective health statuses.
func (r *server) Status(ctx context.Context, req *pb.StatusRequest) (resp *pb.StatusResponse, err error) {
	resp = &pb.StatusResponse{Status: &pb.SystemStatus{Status: pb.SystemStatus_Unknown}}

	resp.Status, err = r.agent.recentStatus()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	setSystemStatus(resp)

	return resp, nil
}

// LocalStatus reports the health status of the local serf node.
func (r *server) LocalStatus(ctx context.Context, req *pb.LocalStatusRequest) (resp *pb.LocalStatusResponse, err error) {
	resp = &pb.LocalStatusResponse{}

	resp.Status, err = r.agent.recentLocalStatus()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return resp, nil
}

// newRPCServer creates an agent RPC endpoint for each provided listener.
func newRPCServer(agent *agent, listeners []net.Listener) *server {
	backend := grpc.NewServer()
	server := &server{agent: agent, Server: backend}
	pb.RegisterAgentServer(backend, server)
	for _, listener := range listeners {
		go backend.Serve(listener)
	}
	return server
}

// defaultDialRPC is a default RPC client factory function.
// It creates a new client based on address details from the specific serf member.
func defaultDialRPC(member *serf.Member) (*client, error) {
	return NewClient(fmt.Sprintf("%s:%d", member.Addr.String(), RPCPort))
}

func setSystemStatus(resp *pb.StatusResponse) {
	var foundMaster bool

	resp.Status.Status = pb.SystemStatus_Running
	for _, node := range resp.Status.Nodes {
		if !foundMaster && isMaster(node.MemberStatus) {
			foundMaster = true
		}
		if resp.Status.Status == pb.SystemStatus_Running {
			resp.Status.Status = nodeToSystemStatus(node.Status)
		}
		if node.MemberStatus.Status == pb.MemberStatus_Failed {
			resp.Status.Status = pb.SystemStatus_Degraded
		}
	}
	if !foundMaster {
		resp.Status.Status = pb.SystemStatus_Degraded
		resp.Summary = errNoMaster.Error()
	}
}

func isMaster(member *pb.MemberStatus) bool {
	value, ok := member.Tags["role"]
	return ok && value == "master"
}

func nodeToSystemStatus(status pb.NodeStatus_Type) pb.SystemStatus_Type {
	switch status {
	case pb.NodeStatus_Running:
		return pb.SystemStatus_Running
	case pb.NodeStatus_Degraded:
		return pb.SystemStatus_Degraded
	default:
		return pb.SystemStatus_Unknown
	}
}
