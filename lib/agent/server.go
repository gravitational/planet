package agent

import (
	"encoding/json"
	"fmt"
	"net"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	"github.com/gravitational/planet/Godeps/_workspace/src/golang.org/x/net/context"
	"github.com/gravitational/planet/Godeps/_workspace/src/google.golang.org/grpc"
	"github.com/gravitational/planet/lib/agent/health"
	pb "github.com/gravitational/planet/lib/agent/proto/agentpb"
)

// This file implements agent RPC server
type rpcServer struct {
	*agent
}

// pb.AgentServiceServer
func (r *rpcServer) Status(ctx context.Context, req *pb.StatusRequest) (*pb.StatusResponse, error) {
	q := &agentQuery{
		Serf: r.Serf(),
		cmd:  string(cmdStatus),
	}
	if err := q.run(); err != nil {
		return nil, trace.Wrap(err, "failed to run serf query")
	}

	// TODO: map serf query response to StatusResponse
	resp := &pb.StatusResponse{}

	members := r.Serf().Members()
	for _, member := range members {
		resp.Nodes = append(resp.Nodes, &pb.Node{
			Name:   member.Name,
			Status: member.Status.String(),
			Tags:   member.Tags,
			Addr:   fmt.Sprintf("%s:%d", member.Addr.String(), member.Port),
		})
	}
	for _, response := range q.responses {
		var nodeStatus health.NodeStatus
		if err := json.Unmarshal(response, &nodeStatus); err != nil {
			return nil, trace.Wrap(err, "failed to unmarshal query result")
		}
		var probes []*pb.Probe
		for _, probe := range nodeStatus.Probes {
			probes = append(probes, &pb.Probe{
				Checker: probe.Checker,
				Extra:   probe.Service,
				Status:  toServiceStatus(probe.Status),
				Error:   probe.Message,
			})
		}
		resp.NodeStatuses = append(resp.NodeStatuses, &pb.NodeStatus{
			Name:   nodeStatus.Name,
			Probes: probes,
		})
	}

	return resp, nil
}

func newRPCServer(agent *agent, listener net.Listener) *rpcServer {
	server := grpc.NewServer()
	rpc := &rpcServer{agent: agent}
	pb.RegisterAgentServiceServer(server, rpc)
	go server.Serve(listener)
	return rpc
}

// FIXME: remove when health uses the proto types
func toServiceStatus(status health.StatusType) pb.ServiceStatus {
	switch status {
	case health.StatusRunning:
		return pb.ServiceStatus_RUNNING
	case health.StatusFailed:
		fallthrough
	default:
		return pb.ServiceStatus_FAILED
	}
}
