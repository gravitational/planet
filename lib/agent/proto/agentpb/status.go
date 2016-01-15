package agentpb

// This file implements JSON encoding/decoding for status types.

// encoding.TextMarshaler
func (s StatusType) MarshalText() (text []byte, err error) {
	switch s {
	case StatusType_SystemRunning:
		return []byte("running"), nil
	case StatusType_SystemDegraded:
		return []byte("degraded"), nil
	}
	return nil, nil
}

// encoding.TextUnmarshaler
func (s *StatusType) UnmarshalText(text []byte) error {
	switch string(text) {
	case "running":
		*s = StatusType_SystemRunning
		return nil
	case "degraded":
		*s = StatusType_SystemDegraded
		return nil
	}
	return nil
}

// encoding.TextMarshaler
func (s ServiceStatusType) MarshalText() (text []byte, err error) {
	switch s {
	case ServiceStatusType_ServiceRunning:
		return []byte("running"), nil
	case ServiceStatusType_ServiceFailed:
		return []byte("failed"), nil
	case ServiceStatusType_ServiceTerminated:
		return []byte("terminated"), nil
	}
	return nil, nil
}

// encoding.TextUnmarshaler
func (s *ServiceStatusType) UnmarshalText(text []byte) error {
	switch string(text) {
	case "running":
		*s = ServiceStatusType_ServiceRunning
	case "failed":
		*s = ServiceStatusType_ServiceFailed
	case "terminated":
		*s = ServiceStatusType_ServiceTerminated
	}
	return nil
}

// encoding.TextMarshaler
func (s MemberStatusType) MarshalText() (text []byte, err error) {
	switch s {
	case MemberStatusType_MemberAlive:
		return []byte("alive"), nil
	case MemberStatusType_MemberLeaving:
		return []byte("leaving"), nil
	case MemberStatusType_MemberLeft:
		return []byte("left"), nil
	case MemberStatusType_MemberFailed:
		return []byte("failed"), nil
	default:
		return []byte("none"), nil
	}
	return nil, nil
}

// encoding.TextUnmarshaler
func (s *MemberStatusType) UnmarshalText(text []byte) error {
	switch string(text) {
	case "alive":
		*s = MemberStatusType_MemberAlive
	case "leaving":
		*s = MemberStatusType_MemberLeaving
	case "left":
		*s = MemberStatusType_MemberLeft
	case "failed":
		*s = MemberStatusType_MemberFailed
	default:
		*s = MemberStatusType_MemberNone
	}
	return nil
}
