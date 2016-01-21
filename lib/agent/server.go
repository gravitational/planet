package agent

import (
	"errors"
	"fmt"
	"net"

	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/log"
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	serf "github.com/gravitational/planet/Godeps/_workspace/src/github.com/hashicorp/serf/client"
	"github.com/gravitational/planet/Godeps/_workspace/src/golang.org/x/net/context"
	"github.com/gravitational/planet/Godeps/_workspace/src/google.golang.org/grpc"
	pb "github.com/gravitational/planet/lib/agent/proto/agentpb"
)

const RPCPort = 7575 // FIXME: use serf to discover agents

var errNoMaster = errors.New("master node unavailable")

// server implements RPC for an agent.
type server struct {
	*grpc.Server
	agent *agent
}

// pb.AgentServiceServer
func (r *server) Status(ctx context.Context, req *pb.StatusRequest) (*pb.StatusResponse, error) {
	resp := &pb.StatusResponse{Status: &pb.SystemStatus{}}

	members, err := r.agent.serfClient.Members()
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
		if r.agent.name == member.Name {
			status, err = r.agent.getStatus()
		} else {
			status, err = r.getStatusFrom(&member)
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

func (r *server) LocalStatus(ctx context.Context, req *pb.LocalStatusRequest) (resp *pb.LocalStatusResponse, err error) {
	resp = &pb.LocalStatusResponse{}

	resp.Status, err = r.agent.getStatus()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	// Update agent cache
	r.agent.cache.UpdateNode(resp.Status)

	return resp, nil
}

func (r *server) getStatusFrom(member *serf.Member) (result *pb.NodeStatus, err error) {
	client, err := r.agent.dialRPC(member)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	defer client.Close()
	var status *pb.NodeStatus
	status, err = client.LocalStatus()
	return status, nil
}

func newRPCServer(agent *agent, listener net.Listener) *server {
	backend := grpc.NewServer()
	server := &server{agent: agent, Server: backend}
	pb.RegisterAgentServer(backend, server)
	go backend.Serve(listener)
	return server
}

func defaultDialRPC(member *serf.Member) (*client, error) {
	return NewClient(fmt.Sprintf("%s:%d", member.Addr.String(), RPCPort))
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
		resp.Summary = errNoMaster.Error()
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
