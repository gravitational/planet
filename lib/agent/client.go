package agent

import (
	pb "github.com/gravitational/planet/lib/agent/proto/agentpb"
	"github.com/gravitational/trace"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

// Client is an interface to communicate with the serf cluster via agent RPC.
type Client interface {
	// Status reports the health status of a serf cluster.
	Status() (*pb.SystemStatus, error)
	// LocalStatus reports the health status of the local serf cluster node.
	LocalStatus() (*pb.NodeStatus, error)
}

type client struct {
	pb.AgentClient
	conn *grpc.ClientConn
}

// NewClient creates a new instance of an agent RPC client to the given address.
func NewClient(addr string) (*client, error) {
	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	c := pb.NewAgentClient(conn)
	return &client{AgentClient: c, conn: conn}, nil
}

// Status reports the health status of the serf cluster.
func (r *client) Status() (*pb.SystemStatus, error) {
	resp, err := r.AgentClient.Status(context.TODO(), &pb.StatusRequest{})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return resp.Status, nil
}

// LocalStatus reports the health status of the local serf node.
func (r *client) LocalStatus() (*pb.NodeStatus, error) {
	resp, err := r.AgentClient.LocalStatus(context.TODO(), &pb.LocalStatusRequest{})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return resp.Status, nil
}

// Close closes the RPC client connection.
func (r *client) Close() error {
	return r.conn.Close()
}
