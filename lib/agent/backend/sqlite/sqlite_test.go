package sqlite

import (
	"testing"
	"time"

	_ "github.com/gravitational/planet/Godeps/_workspace/src/github.com/mattn/go-sqlite3"
	pb "github.com/gravitational/planet/lib/agent/proto/agentpb"
)

const node = "node-1"
const anotherNode = "node-2"

func TestAddsStats(t *testing.T) {
	backend, err := newTestBackend()
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()

	ts := time.Now()
	status := newStatus(node, ts)
	err = backend.UpdateNode(status)
	if err != nil {
		t.Fatal(err)
	}

	var count int64
	when := (*timestamp)(pb.TimeToProto(ts))
	if err = backend.QueryRow(`SELECT COUNT(*) FROM probe WHERE captured_at = ?`, when).Scan(&count); err != nil {
		t.Error(err)
	}
	if count != 2 {
		t.Errorf("expected 2 probes but got %d", count)
	}
}

func TestDeletesOlderStats(t *testing.T) {
	backend, err := newTestBackend()
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()

	ts := time.Now().Add(-scavengeTimeout)
	status := newStatus(anotherNode, ts)
	err = backend.UpdateNode(status)
	if err != nil {
		t.Fatal(err)
	}

	if err = backend.deleteOlderThan(ts.Add(1 * time.Second)); err != nil {
		t.Error(err)
	}

	var count int64
	when := (*timestamp)(pb.TimeToProto(ts))
	if err = backend.QueryRow(`SELECT COUNT(*) FROM probe WHERE captured_at = ?`, when).Scan(&count); err != nil {
		t.Error(err)
	}
	if count != 0 {
		t.Errorf("expected no probes for %s but got %d", when, count)
	}
}

func TestGetsRecentStats(t *testing.T) {
	backend, err := newTestBackend()
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()

	if err = addStatsForNode(t, backend, node); err != nil {
		t.Fatal(err)
	}
	if err = addStatsForNode(t, backend, anotherNode); err != nil {
		t.Fatal(err)
	}

	probes, err := backend.RecentStatus(node)
	if err != nil {
		t.Fatal(err)
	}
	if len(probes) != 5 {
		t.Errorf("expected 5 probes but got %d", len(probes))
	}
}

func addStatsForNode(t *testing.T, b *backend, node string) error {
	baseTime := baseTime(t)
	for i := 0; i < 3; i++ {
		status := newStatus(node, baseTime)
		if err := b.UpdateNode(status); err != nil {
			return err
		}
		baseTime = baseTime.Add(10 * time.Second)
	}
	return nil
}

func newTestBackend() (*backend, error) {
	backend, err := newInMemory()
	if err != nil {
		return nil, err
	}
	return backend, nil
}

func newStatus(name string, time time.Time) *pb.NodeStatus {
	when := pb.TimeToProto(time)
	status := &pb.NodeStatus{
		Name: name,
		Probes: []*pb.Probe{
			&pb.Probe{
				Checker:   "foo",
				Status:    pb.ServiceStatusType_ServiceFailed,
				Error:     "cannot lift weights",
				Timestamp: when,
			},
			&pb.Probe{
				Checker:   "bar",
				Status:    pb.ServiceStatusType_ServiceFailed,
				Error:     "cannot get up",
				Timestamp: when,
			},
		},
	}
	return status
}

func (ts timestamp) String() string {
	result, _ := pb.Timestamp(ts).MarshalText()
	return string(result)
}

func baseTime(t *testing.T) time.Time {
	time, err := time.Parse(time.RFC3339, "2001-01-01T11:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	return time
}
