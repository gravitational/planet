package agent

import (
	"github.com/gravitational/planet/Godeps/_workspace/src/github.com/gravitational/trace"
	"github.com/gravitational/planet/Godeps/_workspace/src/golang.org/x/net/context"
	"github.com/gravitational/planet/Godeps/_workspace/src/google.golang.org/grpc"
	"github.com/gravitational/planet/lib/agent/health"
	pb "github.com/gravitational/planet/lib/agent/proto/agentpb"
)

// Client is an interface to communicate with the serf cluster.
type Client interface {
	Status() (*health.Status, error)
}

type client struct {
	pb.AgentServiceClient
}

func NewClient(addr string) (*client, error) {
	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	c := pb.NewAgentServiceClient(conn)
	return &client{c}, nil
}

// Status reports the status of the serf cluster.
func (r *client) Status() (*health.Status, error) {
	resp, err := r.AgentServiceClient.Status(context.Background(), &pb.StatusRequest{})
	if err != nil {
		return nil, trace.Wrap(err)
	}

	// FIXME: return the results of AgentServiceClient.Status directly
	var status health.Status
	// var healthyNodes []string
	for _, ns := range resp.NodeStatuses {
		var nodeStatus health.NodeStatus
		nodeStatus.Name = ns.Name
		for _, probe := range ns.Probes {
			nodeStatus.Probes = append(nodeStatus.Probes, health.Probe{
				Checker: probe.Checker,
				Service: probe.Extra,
				Message: probe.Error,
				Status:  toStatus(probe.Status),
			})
		}
		status.Nodes = append(status.Nodes, nodeStatus)
	}
	return &status, nil
}

func toStatus(status pb.ServiceStatus) health.StatusType {
	switch status {
	case pb.ServiceStatus_RUNNING:
		return health.StatusRunning
	case pb.ServiceStatus_FAILED:
		fallthrough
	default:
		return health.StatusFailed
	}
}
