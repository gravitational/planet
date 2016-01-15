package agent

import (
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	"github.com/gravitational/planet/Godeps/_workspace/src/golang.org/x/net/context"
	"github.com/gravitational/planet/Godeps/_workspace/src/google.golang.org/grpc"
	pb "github.com/gravitational/planet/lib/agent/proto/agentpb"
)

// Client is an interface to communicate with the serf cluster via agent RPC.
type Client interface {
	Status() (*pb.SystemStatus, error)
	LocalStatus() (*pb.NodeStatus, error)
}

type client struct {
	pb.AgentServiceClient
}

var _ Client = (*client)(nil)

func NewClient(addr string) (*client, error) {
	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	c := pb.NewAgentServiceClient(conn)
	return &client{c}, nil
}

// Status reports the status of the serf cluster.
func (r *client) Status() (*pb.SystemStatus, error) {
	// FIXME: implement proper timeouts and cancellation
	resp, err := r.AgentServiceClient.Status(context.Background(), &pb.StatusRequest{})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return resp.Status, nil
}

// Status reports the status of the specific serf node.
func (r *client) LocalStatus() (*pb.NodeStatus, error) {
	// FIXME: implement proper timeouts and cancellation
	resp, err := r.AgentServiceClient.LocalStatus(context.Background(), &pb.LocalStatusRequest{})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return resp.Status, nil
}
