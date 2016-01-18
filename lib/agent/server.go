package agent

import (
	"fmt"
	"net"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	"github.com/gravitational/planet/Godeps/_workspace/src/golang.org/x/net/context"
	"github.com/gravitational/planet/Godeps/_workspace/src/google.golang.org/grpc"
	pb "github.com/gravitational/planet/lib/agent/proto/agentpb"
)

// RPCServer is the interface implemented by the agent RPC server.
type RPCServer interface {
	Stop()
}

const RPCPort = 7575 // FIXME: use serf to discover agent

// server implements RPC for an agent.
type server struct {
	*agent
}

// pb.AgentServiceServer
func (r *server) Status(ctx context.Context, req *pb.StatusRequest) (*pb.StatusResponse, error) {
	resp := &pb.StatusResponse{Status: &pb.SystemStatus{}}

	members, err := r.serfClient.Members()
	if err != nil {
		return nil, trace.Wrap(err, "failed to query serf members")
	}

	for _, member := range members {
		resp.Status.Members = append(resp.Status.Members, &pb.MemberStatus{
			Name:   member.Name,
			Status: toMemberStatus(member.Status),
			Tags:   member.Tags,
			Addr:   fmt.Sprintf("%s:%d", member.Addr.String(), member.Port),
		})
		var status *pb.NodeStatus
		if r.name == member.Name {
			status, err = r.getStatus()
		} else {
			status, err = r.getStatusFrom(member.Addr)
		}
		if err != nil {
			log.Errorf("failed to query status of serf node %s (%v)", member.Name, member.Addr)
		} else {
			// Update agent cache
			r.agent.cache.UpdateNode(status)
			resp.Status.Nodes = append(resp.Status.Nodes, status)
		}
	}
	setSystemStatus(resp)

	return resp, nil
}

func (r *server) LocalStatus(ctx context.Context, req *pb.LocalStatusRequest) (*pb.LocalStatusResponse, error) {
	resp := &pb.LocalStatusResponse{Status: &pb.NodeStatus{}}

	status, err := r.agent.getStatus()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	// Update agent cache
	r.agent.cache.UpdateNode(status)

	return resp, nil
}

func (r *server) getStatusFrom(addr net.IP) (result *pb.NodeStatus, err error) {
	client, err := NewClient(fmt.Sprintf("%s:%d", addr.String(), RPCPort))
	if err != nil {
		return nil, trace.Wrap(err)
	}
	var status *pb.NodeStatus
	status, err = client.LocalStatus()
	return status, nil
}

func newRPCServer(agent *agent, listener net.Listener) *grpc.Server {
	backend := grpc.NewServer()
	server := &server{agent: agent}
	pb.RegisterAgentServer(backend, server)
	go backend.Serve(listener)
	return backend
}

func setSystemStatus(resp *pb.StatusResponse) {
	var foundMaster bool

	resp.Status.Status = pb.SystemStatus_Running
	for _, member := range resp.Status.Members {
		if member.Status == pb.MemberStatus_Failed {
			resp.Status.Status = pb.SystemStatus_Degraded
		}
		if !foundMaster && isMaster(member) {
			foundMaster = true
		}
	}
	for _, node := range resp.Status.Nodes {
		resp.Status.Status = pb.SystemStatus_Type(node.Status)
		if node.Status != pb.NodeStatus_Running {
			break
		}
	}
	if !foundMaster {
		resp.Status.Status = pb.SystemStatus_Degraded
		resp.Summary = "master node unavailable"
	}
}

func isMaster(member *pb.MemberStatus) bool {
	value, ok := member.Tags["role"]
	return ok && value == "master"
}

func toMemberStatus(status string) pb.MemberStatus_Type {
	switch status {
	case "alive":
		return pb.MemberStatus_Alive
	case "leaving":
		return pb.MemberStatus_Leaving
	case "left":
		return pb.MemberStatus_Left
	case "failed":
		return pb.MemberStatus_Failed
	}
	return pb.MemberStatus_None
}
