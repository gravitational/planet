package agentpb

import (
	"bytes"
	"os"
	"testing"

	log "github.com/Sirupsen/logrus"
	. "gopkg.in/check.v1"
)

func init() {
	log.SetOutput(os.Stderr)
	log.SetLevel(log.InfoLevel)
}

func TestProtoStatus(t *testing.T) { TestingT(t) }

type ProtoStatus struct{}

var _ = Suite(&ProtoStatus{})

func (r *ProtoStatus) TestMarshalsMemberStatus(c *C) {
	var tests = []struct {
		status   MemberStatus_Type
		expected []byte
	}{
		{
			status:   MemberStatus_Alive,
			expected: []byte("alive"),
		},
		{
			status:   MemberStatus_Leaving,
			expected: []byte("leaving"),
		},
		{
			status:   MemberStatus_Left,
			expected: []byte("left"),
		},
		{
			status:   MemberStatus_Failed,
			expected: []byte("failed"),
		},
		{
			status:   MemberStatus_None,
			expected: nil,
		},
	}

	for _, test := range tests {
		text, err := test.status.MarshalText()
		c.Assert(err, IsNil)
		if !bytes.Equal(text, test.expected) {
			c.Errorf("expected %s but got %s", test.expected, text)
		}
		status := new(MemberStatus_Type)
		err = status.UnmarshalText(text)
		c.Assert(err, IsNil)
		c.Assert(test.status, DeepEquals, *status)
	}
}

func (r *ProtoStatus) TestMarshalsSystemStatus(c *C) {
	var tests = []struct {
		status   SystemStatus_Type
		expected []byte
	}{
		{
			status:   SystemStatus_Running,
			expected: []byte("running"),
		},
		{
			status:   SystemStatus_Degraded,
			expected: []byte("degraded"),
		},
		{
			status:   SystemStatus_Unknown,
			expected: nil,
		},
	}

	for _, test := range tests {
		text, err := test.status.MarshalText()
		c.Assert(err, IsNil)
		if !bytes.Equal(text, test.expected) {
			c.Errorf("expected %s but got %s", test.expected, text)
		}
		status := new(SystemStatus_Type)
		err = status.UnmarshalText(text)
		c.Assert(err, IsNil)
		c.Assert(test.status, DeepEquals, *status)
	}
}

func (r *ProtoStatus) TestMarshalsNodeStatus(c *C) {
	var tests = []struct {
		status   NodeStatus_Type
		expected []byte
	}{
		{
			status:   NodeStatus_Running,
			expected: []byte("healthy"), // FIXME: "running"
		},
		{
			status:   NodeStatus_Degraded,
			expected: []byte("degraded"),
		},
		{
			status:   NodeStatus_Unknown,
			expected: nil,
		},
	}

	for _, test := range tests {
		text, err := test.status.MarshalText()
		c.Assert(err, IsNil)
		if !bytes.Equal(text, test.expected) {
			c.Errorf("expected %s but got %s", test.expected, text)
		}
		status := new(NodeStatus_Type)
		err = status.UnmarshalText(text)
		c.Assert(err, IsNil)
		c.Assert(test.status, DeepEquals, *status)
	}
}

func (r *ProtoStatus) TestClonesSystemStatus(c *C) {
	members := [2]*MemberStatus{
		&MemberStatus{
			Name:   "foo",
			Status: MemberStatus_Alive,
			Addr:   "127.0.0.1:7496",
			Tags:   map[string]string{"role": "master", "key0": "value0"},
		},
		&MemberStatus{
			Name:   "bar",
			Status: MemberStatus_Alive,
			Addr:   "127.0.0.2:7496",
			Tags:   map[string]string{"role": "node", "key": "value"},
		},
	}
	probes := []*Probe{
		&Probe{
			Checker: "c1",
			Status:  Probe_Running,
		},
		&Probe{
			Checker: "c2",
			Status:  Probe_Failed,
			Error:   "failed",
		},
	}
	nodes := []*NodeStatus{
		&NodeStatus{
			Name:         "foo",
			Status:       NodeStatus_Running,
			MemberStatus: members[0],
			Probes:       probes,
		},
		&NodeStatus{
			Name:         "bar",
			Status:       NodeStatus_Running,
			MemberStatus: members[1],
			Probes:       probes,
		},
	}
	status := &SystemStatus{
		Nodes: nodes,
	}
	statusCopy := status.Clone()
	c.Assert(status, DeepEquals, statusCopy)
}
