package agent

import (
	"encoding/json"
	"fmt"
	"net"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	"github.com/gravitational/planet/Godeps/_workspace/src/golang.org/x/net/context"
	"github.com/gravitational/planet/Godeps/_workspace/src/google.golang.org/grpc"
	pb "github.com/gravitational/planet/lib/agent/proto/agentpb"
)

// server implements RPC for an agent.
type server struct {
	*agent
}

// pb.AgentServiceServer
func (r *server) Status(ctx context.Context, req *pb.StatusRequest) (*pb.StatusResponse, error) {
	q := &agentQuery{
		Serf: r.Serf(),
		cmd:  string(cmdStatus),
	}
	if err := q.run(); err != nil {
		return nil, trace.Wrap(err, "failed to run serf query")
	}

	resp := &pb.StatusResponse{Status: &pb.SystemStatus{}}

	members := r.Serf().Members()
	for _, member := range members {
		resp.Status.Nodes = append(resp.Status.Nodes, &pb.Node{
			Name:   member.Name,
			Status: pb.MemberStatusType(member.Status),
			Tags:   member.Tags,
			Addr:   fmt.Sprintf("%s:%d", member.Addr.String(), member.Port),
		})
	}
	for _, response := range q.responses {
		status := &pb.NodeStatus{}
		if err := json.Unmarshal(response, &status); err != nil {
			// FIXME: do not fail on not well-formed response?
			return nil, trace.Wrap(err, "failed to unmarshal query result")
		}
		// Update agent cache
		r.agent.cache.UpdateNode(status)
		resp.Status.NodeStatuses = append(resp.Status.NodeStatuses, status)
	}
	setSystemStatus(resp)

	return resp, nil
}

func newRPCServer(agent *agent, listener net.Listener) *server {
	backend := grpc.NewServer()
	server := &server{agent: agent}
	pb.RegisterAgentServiceServer(backend, server)
	go backend.Serve(listener)
	return server
}

func setSystemStatus(resp *pb.StatusResponse) {
	var foundMaster bool

	resp.Status.Status = pb.StatusType_SystemRunning
	for _, member := range resp.Status.Nodes {
		if member.Status == pb.MemberStatusType_MemberFailed {
			resp.Status.Status = pb.StatusType_SystemDegraded
		}
		if value, ok := member.Tags["role"]; !foundMaster && ok && value == "master" {
			foundMaster = true
		}
	}
	for _, node := range resp.Status.NodeStatuses {
		resp.Status.Status = node.Status
		if node.Status != pb.StatusType_SystemRunning {
			break
		}
	}
	if !foundMaster {
		resp.Status.Status = pb.StatusType_SystemDegraded
		resp.Summary = "master node unavailable"
	}
}
