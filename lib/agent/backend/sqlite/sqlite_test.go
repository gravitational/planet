package sqlite

import (
	"os"
	"testing"
	"time"

	log "github.com/Sirupsen/logrus"
	pb "github.com/gravitational/planet/lib/agent/proto/agentpb"
	"github.com/jonboulle/clockwork"
	_ "github.com/mattn/go-sqlite3"
	. "gopkg.in/check.v1"
)

func init() {
	if testing.Verbose() {
		log.SetOutput(os.Stderr)
		log.SetLevel(log.InfoLevel)
	}
}

var nodes = []string{"node-1", "node-2"}

func TestBackend(t *testing.T) { TestingT(t) }

type BackendSuite struct {
	backend *backend
}

type BackendWithClockSuite struct {
	backend *backend
	clock   clockwork.FakeClock
}

var _ = Suite(&BackendSuite{})
var _ = Suite(&BackendWithClockSuite{})

func (r *BackendSuite) SetUpTest(c *C) {
	var err error
	r.backend, err = newTestBackend()
	c.Assert(err, IsNil)
}

func (r *BackendSuite) TearDownTest(c *C) {
	if r.backend != nil {
		r.backend.Close()
	}
}

func (r *BackendWithClockSuite) SetUpTest(c *C) {
	var err error
	r.clock = clockwork.NewFakeClock()
	r.backend, err = newTestBackendWithClock(r.clock)
	c.Assert(err, IsNil)
}

func (r *BackendWithClockSuite) TearDownTest(c *C) {
	r.backend.Close()
}

func (r *BackendSuite) TestUpdatesStatus(c *C) {
	ts := time.Now()
	status := newStatus(nodes, ts)
	c.Assert(r.backend.UpdateStatus(status), IsNil)

	var count int64
	when := timestamp(pb.TimeToProto(ts))
	err := r.backend.QueryRow(`SELECT count(*) FROM system_snapshot WHERE captured_at = ?`, &when).Scan(&count)
	c.Assert(err, IsNil)

	c.Assert(count, Equals, int64(1))
}

func (r *BackendSuite) TestExplicitlyDeletesOlderStats(c *C) {
	ts := time.Now().Add(-scavengeTimeout)
	status := newStatus(nodes, ts)
	err := r.backend.UpdateStatus(status)
	c.Assert(err, IsNil)

	err = r.backend.deleteOlderThan(ts.Add(time.Second))
	c.Assert(err, IsNil)

	var count int64
	when := timestamp(pb.TimeToProto(ts))
	err = r.backend.QueryRow(`SELECT count(*) FROM system_snapshot WHERE captured_at = ?`, when).Scan(&count)
	c.Assert(err, IsNil)

	c.Assert(count, Equals, int64(0))
}

func (r *BackendSuite) TestObtainsRecentStatus(c *C) {
	clock := clockwork.NewFakeClock()

	time := clock.Now()
	status := newStatus(nodes, time)
	status.Status = pb.SystemStatus_Unknown // status not persisted
	c.Assert(r.backend.UpdateStatus(status), IsNil)

	actualStatus, err := r.backend.RecentStatus()
	c.Assert(err, IsNil)

	c.Assert(actualStatus, DeepEquals, status)
}

func (r *BackendWithClockSuite) TestScavengesOlderStats(c *C) {
	c.Assert(updateStatus(r.backend, nodes, r.clock), IsNil)

	r.clock.BlockUntil(1)
	r.clock.Advance(scavengeTimeout + time.Second)
	// block until the scavenge loop goes on another wait round
	r.clock.BlockUntil(1)
	status, err := r.backend.RecentStatus()
	c.Assert(err, IsNil)

	c.Assert(status, IsNil)
}

func updateStatus(b *backend, nodes []string, clock clockwork.Clock) error {
	baseTime := clock.Now()
	for i := 0; i < 3; i++ {
		status := newStatus(nodes, baseTime)
		if err := b.UpdateStatus(status); err != nil {
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

func newStatus(names []string, time time.Time) *pb.SystemStatus {
	when := pb.NewTimeToProto(time)
	var nodes []*pb.NodeStatus
	for _, name := range names {
		nodes = append(nodes, &pb.NodeStatus{
			Name:   name,
			Status: pb.NodeStatus_Degraded,
			MemberStatus: &pb.MemberStatus{
				Name:   name,
				Status: pb.MemberStatus_Alive,
				Tags:   map[string]string{"key": "value", "key2": "value2"},
			},
			Probes: []*pb.Probe{
				&pb.Probe{
					Checker: "foo",
					Status:  pb.Probe_Failed,
					Error:   "cannot lift weights",
				},
				&pb.Probe{
					Checker: "bar",
					Status:  pb.Probe_Failed,
					Error:   "cannot get up",
				},
			},
		})
	}
	status := &pb.SystemStatus{
		Status:    pb.SystemStatus_Degraded,
		Timestamp: when,
		Nodes:     nodes,
	}
	return status
}
