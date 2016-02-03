package agentpb

// This file implements JSON encoding/decoding for status types
// as well as copying functions.

// encoding.TextMarshaler
func (s SystemStatus_Type) MarshalText() (text []byte, err error) {
	switch s {
	case SystemStatus_Running:
		return []byte("running"), nil
	case SystemStatus_Degraded:
		return []byte("degraded"), nil
	default:
		return nil, nil
	}
}

// encoding.TextUnmarshaler
func (s *SystemStatus_Type) UnmarshalText(text []byte) error {
	switch string(text) {
	case "running":
		*s = SystemStatus_Running
	case "degraded":
		*s = SystemStatus_Degraded
	default:
		*s = SystemStatus_Unknown
	}
	return nil
}

// encoding.TextMarshaler
func (s NodeStatus_Type) MarshalText() (text []byte, err error) {
	switch s {
	case NodeStatus_Running:
		// FIXME: unique for backwards-compatibility
		// will be changed to `running`
		return []byte("healthy"), nil // "running"
	case NodeStatus_Degraded:
		return []byte("degraded"), nil
	default:
		return nil, nil
	}
}

// encoding.TextUnmarshaler
func (s *NodeStatus_Type) UnmarshalText(text []byte) error {
	switch string(text) {
	// FIXME: unique for backwards-compatibility
	// will be changed to `running`
	case "healthy": // "running"
		*s = NodeStatus_Running
	case "degraded":
		*s = NodeStatus_Degraded
	default:
		*s = NodeStatus_Unknown
	}
	return nil
}

// encoding.TextMarshaler
func (s Probe_Type) MarshalText() (text []byte, err error) {
	switch s {
	case Probe_Running:
		// FIXME: unique for backwards-compatibility
		// will be changed to `running`
		return []byte("healthy"), nil // "running"
	case Probe_Failed:
		return []byte("failed"), nil
	case Probe_Terminated:
		return []byte("terminated"), nil
	default:
		return nil, nil
	}
}

// encoding.TextUnmarshaler
func (s *Probe_Type) UnmarshalText(text []byte) error {
	switch string(text) {
	// FIXME: unique for backwards-compatibility
	// will be changed to `running`
	case "healthy": // "running"
		*s = Probe_Running
	case "failed":
		*s = Probe_Failed
	case "terminated":
		*s = Probe_Terminated
	default:
		*s = Probe_Unknown
	}
	return nil
}

// encoding.TextMarshaler
func (s MemberStatus_Type) MarshalText() (text []byte, err error) {
	switch s {
	case MemberStatus_Alive:
		return []byte("alive"), nil
	case MemberStatus_Leaving:
		return []byte("leaving"), nil
	case MemberStatus_Left:
		return []byte("left"), nil
	case MemberStatus_Failed:
		return []byte("failed"), nil
	case MemberStatus_None:
	default:
		return []byte("none"), nil
	}
	return nil, nil
}

// encoding.TextUnmarshaler
func (s *MemberStatus_Type) UnmarshalText(text []byte) error {
	switch string(text) {
	case "alive":
		*s = MemberStatus_Alive
	case "leaving":
		*s = MemberStatus_Leaving
	case "left":
		*s = MemberStatus_Left
	case "failed":
		*s = MemberStatus_Failed
	default:
		*s = MemberStatus_None
	}
	return nil
}

// Clone returns a copy of this NodeStatus value.
func (r *NodeStatus) Clone() (result *NodeStatus) {
	result = new(NodeStatus)
	*result = *r
	if r.MemberStatus != nil {
		result.MemberStatus = r.MemberStatus.Clone()
	}
	for i, probe := range r.Probes {
		result.Probes[i] = probe.Clone()
	}
	return result
}

// Clone returns a copy of this SystemStatus value.
func (r *SystemStatus) Clone() (result *SystemStatus) {
	result = new(SystemStatus)
	*result = *r
	for i, node := range r.Nodes {
		result.Nodes[i] = node.Clone()
	}
	return result
}

// Clone returns a copy of this Probe value.
func (r *Probe) Clone() (result *Probe) {
	result = new(Probe)
	*result = *r
	if r.Timestamp != nil {
		result.Timestamp = r.Timestamp.Clone()
	}
	return result
}

// Clone returns a copy of this MemberStatus value.
func (r *MemberStatus) Clone() (result *MemberStatus) {
	result = new(MemberStatus)
	*result = *r
	return result
}
