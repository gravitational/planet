package sqlite

import (
	"testing"
	"time"

	pb "github.com/gravitational/planet/lib/agent/proto/agentpb"
	"github.com/jonboulle/clockwork"
	_ "github.com/mattn/go-sqlite3"
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
	when := timestamp(pb.TimeToProto(ts))
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
	when := timestamp(pb.TimeToProto(ts))
	if err = backend.QueryRow(`SELECT COUNT(*) FROM probe WHERE captured_at = ?`, when).Scan(&count); err != nil {
		t.Error(err)
	}
	if count != 0 {
		t.Errorf("expected no probes for %s but got %d", when, count)
	}
}

func TestGetsRecentStats(t *testing.T) {
	clock := clockwork.NewFakeClock()
	backend, err := newTestBackend()
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()

	if err = addStatsForNode(t, backend, node, clock); err != nil {
		t.Fatal(err)
	}
	if err = addStatsForNode(t, backend, anotherNode, clock); err != nil {
		t.Fatal(err)
	}

	expected := 5
	probes, err := backend.RecentStatus(node)
	if err != nil {
		t.Fatal(err)
	}
	if len(probes) != expected {
		t.Errorf("expected %d probes but got %d", expected, len(probes))
	}
}

func TestScavengesOlderStats(t *testing.T) {
	clock := clockwork.NewFakeClock()
	backend, err := newTestBackendWithClock(clock)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()

	if err := addStatsForNode(t, backend, node, clock); err != nil {
		t.Fatal(err)
	}
	clock.BlockUntil(1)
	clock.Advance(scavengeTimeout + 1*time.Second)
	// block until the scavenge loop goes on another wait round
	clock.BlockUntil(1)
	probes, err := backend.RecentStatus(node)
	if err != nil {
		t.Fatal(err)
	}
	if len(probes) > 0 {
		t.Errorf("expected no stats but got %d", len(probes))
	}
}

func addStatsForNode(t *testing.T, b *backend, node string, clock clockwork.Clock) error {
	baseTime := clock.Now()
	for i := 0; i < 3; i++ {
		status := newStatus(node, baseTime)
		if err := b.UpdateNode(status); err != nil {
			return err
		}
		baseTime = baseTime.Add(-10 * time.Second)
	}
	return nil
}

func newTestBackend() (*backend, error) {
	return newTestBackendWithClock(clockwork.NewRealClock())
}

func newTestBackendWithClock(clock clockwork.Clock) (*backend, error) {
	backend, err := newInMemory(clock)
	if err != nil {
		return nil, err
	}
	return backend, nil
}

func newStatus(name string, time time.Time) *pb.NodeStatus {
	when := pb.NewTimeToProto(time)
	status := &pb.NodeStatus{
		Name: name,
		Probes: []*pb.Probe{
			&pb.Probe{
				Checker:   "foo",
				Status:    pb.Probe_Failed,
				Error:     "cannot lift weights",
				Timestamp: when,
			},
			&pb.Probe{
				Checker:   "bar",
				Status:    pb.Probe_Failed,
				Error:     "cannot get up",
				Timestamp: when,
			},
		},
	}
	return status
}
