package agentpb

import (
	"bytes"
	"testing"
)

func TestMarshalsMemberStatus(t *testing.T) {
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
		if err != nil {
			t.Error(err)
		}
		if !bytes.Equal(text, test.expected) {
			t.Errorf("expected %s but got %s", test.expected, text)
		}
		status := new(MemberStatus_Type)
		err = status.UnmarshalText(text)
		if err != nil {
			t.Error(err)
		}
		if test.status != *status {
			t.Errorf("expected %s to equal %s", test.status, status)
		}
	}
}

func TestMarshalsSystemStatus(t *testing.T) {
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
		if err != nil {
			t.Error(err)
		}
		if !bytes.Equal(text, test.expected) {
			t.Errorf("expected %s but got %s", test.expected, text)
		}
		status := new(SystemStatus_Type)
		err = status.UnmarshalText(text)
		if err != nil {
			t.Error(err)
		}
		if test.status != *status {
			t.Errorf("expected %s to equal %s", test.status, status)
		}
	}
}

func TestMarshalsNodeStatus(t *testing.T) {
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
		if err != nil {
			t.Error(err)
		}
		if !bytes.Equal(text, test.expected) {
			t.Errorf("expected %s but got %s", test.expected, text)
		}
		status := new(NodeStatus_Type)
		err = status.UnmarshalText(text)
		if err != nil {
			t.Error(err)
		}
		if test.status != *status {
			t.Errorf("expected %s to equal %s", test.status, status)
		}
	}
}
