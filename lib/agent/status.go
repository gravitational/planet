package agent

import (
	"errors"

	pb "github.com/gravitational/planet/lib/agent/proto/agentpb"
)

var errNoMaster = errors.New("master node unavailable")

// setSystemStatus combines the status of individual nodes into the status of the
// cluster as a whole.
// It additionally augments the cluster status with human-readable summary.
func setSystemStatus(status *pb.SystemStatus) {
	var foundMaster bool

	status.Status = pb.SystemStatus_Running
	for _, node := range status.Nodes {
		if !foundMaster && isMaster(node.MemberStatus) {
			foundMaster = true
		}
		if status.Status == pb.SystemStatus_Running {
			status.Status = nodeToSystemStatus(node.Status)
		}
		if node.MemberStatus.Status == pb.MemberStatus_Failed {
			status.Status = pb.SystemStatus_Degraded
		}
	}
	if !foundMaster {
		status.Status = pb.SystemStatus_Degraded
		status.Summary = errNoMaster.Error()
	}
}

func isMaster(member *pb.MemberStatus) bool {
	value, ok := member.Tags["role"]
	return ok && value == string(RoleMaster)
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
