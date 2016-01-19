package agentpb

import "testing"

func TestMarshalsMemberStatus(t *testing.T) {
	s := MemberStatus_Alive
	text, err := s.MarshalText()
	if err != nil {
		t.Error(err)
	}
	expected := "alive"
	if string(text) != expected {
		t.Errorf("expected %s but got %s", expected, string(text))
	}
	s2 := new(MemberStatus_Type)
	err = s2.UnmarshalText(text)
	if err != nil {
		t.Error(err)
	}
	if s != *s2 {
		t.Errorf("expected %s to equal %s", s, s2)
	}
}

func TestMarshalsSystemStatus(t *testing.T) {
	s := SystemStatus_Degraded
	text, err := s.MarshalText()
	if err != nil {
		t.Error(err)
	}
	expected := "degraded"
	if string(text) != expected {
		t.Errorf("expected %s but got %s", expected, string(text))
	}
	s2 := new(SystemStatus_Type)
	err = s2.UnmarshalText(text)
	if err != nil {
		t.Error(err)
	}
	if s != *s2 {
		t.Errorf("expected %s to equal %s", s, s2)
	}
}

func TestMarshalsNodeStatus(t *testing.T) {
	s := NodeStatus_Running
	text, err := s.MarshalText()
	if err != nil {
		t.Error(err)
	}
	expected := "running"
	if string(text) != expected {
		t.Errorf("expected %s but got %s", expected, string(text))
	}
	s2 := new(NodeStatus_Type)
	err = s2.UnmarshalText(text)
	if err != nil {
		t.Error(err)
	}
	if s != *s2 {
		t.Errorf("expected %s to equal %s", s, s2)
	}
}
