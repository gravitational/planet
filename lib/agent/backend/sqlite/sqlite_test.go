package sqlite

import (
	"testing"
	"time"

	_ "github.com/gravitational/planet/Godeps/_workspace/src/github.com/mattn/go-sqlite3"
	pb "github.com/gravitational/planet/lib/agent/proto/agentpb"
)

func TestUpdatesOrGetsNode(t *testing.T) {
	backend, cleanup, err := newBackendAndTx()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	var id int64
	if id, err = backend.upsertNode("node-1"); err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Error("failed to create a node")
	}
}

func TestAddsStats(t *testing.T) {
	backend, cleanup, err := newBackendAndTx()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	when := (*timestamp)(pb.TimeToProto(time.Now()))
	status := newStatus("node-1", when)
	err = backend.UpdateNode(status)
	if err != nil {
		t.Fatal(err)
	}

	var count int64
	if err = backend.Get(&count, `SELECT COUNT(*) FROM probe WHERE captured_at = ?`, when); err != nil {
		t.Error(err)
	}
	if count != 2 {
		t.Errorf("expected 2 probes but got %d", count)
	}
}

func TestDeletesOlderStats(t *testing.T) {
	backend, cleanup, err := newBackendAndTx()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	ts := time.Now().Add(-scavengeTimeout)
	when := (*timestamp)(pb.TimeToProto(ts))
	status := newStatus("node-2", when)
	err = backend.UpdateNode(status)
	if err != nil {
		t.Fatal(err)
	}

	if err = backend.deleteOlderThan(ts.Add(1 * time.Second)); err != nil {
		t.Error(err)
	}

	var count int64
	if err = backend.Get(&count, `SELECT COUNT(*) FROM probe WHERE captured_at = ?`, when); err != nil {
		t.Error(err)
	}
	if count != 0 {
		t.Errorf("expected no probes for given timestamp but got %d", count)
	}
}

func TestGetsRecentStats(t *testing.T) {
	backend, cleanup, err := newBackendAndTx()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	probes, err := backend.RecentStatus("node-1")
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("probes: %v", probes)
}

func newBackendAndTx() (*backend, cleanup, error) {
	// FIXME: use in-memory constructor
	backend, err := New("tmp")
	if err != nil {
		return nil, nil, err
	}
	tx, err := backend.Beginx()
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		backend.Close()
		if err != nil {
			tx.Rollback()
			return
		}
		err = tx.Commit()
	}
	return backend, cleanup, nil
}

func newStatus(name string, when *timestamp) *pb.NodeStatus {
	status := &pb.NodeStatus{
		Name: name,
		Probes: []*pb.Probe{
			&pb.Probe{
				Checker:   "foo",
				Status:    pb.ServiceStatusType_ServiceFailed,
				Error:     "cannot lift weights",
				Timestamp: (*pb.Timestamp)(when),
			},
			&pb.Probe{
				Checker:   "bar",
				Status:    pb.ServiceStatusType_ServiceFailed,
				Error:     "cannot get up",
				Timestamp: (*pb.Timestamp)(when),
			},
		},
	}
	return status
}
