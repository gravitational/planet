package agentpb

import (
	"testing"
	"time"
)

func TestWrapsTime(t *testing.T) {
	expected := time.Now().UTC()
	ts := TimeToProto(expected)
	actual := ts.ToTime()
	if !actual.Equal(expected) {
		t.Fatalf("expected %v to equal %v", actual, expected)
	}
}

func TestMarshalsTime(t *testing.T) {
	expected := time.Now().UTC()
	ts := TimeToProto(expected)
	text, err := ts.MarshalText()
	if err != nil {
		t.Fatal(err)
	}
	ts2 := Timestamp{}
	err = ts2.UnmarshalText(text)
	if err != nil {
		t.Fatal(err)
	}
	if !ts.Equal(ts2) {
		t.Fatalf("expected %v to equal %v", ts.String(), ts2.String())
	}
}
