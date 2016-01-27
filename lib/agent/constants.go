package agent

// MemberStatus describes the state of a serf node.
type MemberStatus string

const (
	MemberAlive   MemberStatus = "alive"
	MemberLeaving              = "leaving"
	MemberLeft                 = "left"
	MemberFailed               = "failed"
)

// Role describes the agent's server role.
type Role string

const (
	RoleMaster Role = "master"
	RoleNode        = "node"
)
