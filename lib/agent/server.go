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

	resp := &pb.StatusResponse{Status: &pb.SystemStatus{}}

	members := r.Serf().Members()
	for _, member := range members {
		resp.Status.Nodes = append(resp.Status.Nodes, &pb.Node{
			Name:   member.Name,
			Status: member.Status.String(),
			Tags:   member.Tags,
			Addr:   fmt.Sprintf("%s:%d", member.Addr.String(), member.Port),
		})
	}
	for _, response := range q.responses {
		ns := &pb.NodeStatus{}
		if err := json.Unmarshal(response, &ns); err != nil {
			return nil, trace.Wrap(err, "failed to unmarshal query result")
		}
		resp.Status.NodeStatuses = append(resp.Status.NodeStatuses, ns)
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
