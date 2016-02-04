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

const node = "node-1"
const anotherNode = "node-2"

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
	r.backend.Close()
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

func (r *BackendSuite) TestAddsStats(c *C) {
	ts := time.Now()
	status := newStatus(node, ts)
	err := r.backend.UpdateNode(status)
	c.Assert(err, IsNil)

	var count int64
	when := timestamp(pb.TimeToProto(ts))
	err = r.backend.QueryRow(`SELECT COUNT(*) FROM probe WHERE captured_at = ?`, when).Scan(&count)
	c.Assert(err, IsNil)

	c.Assert(count, Equals, int64(2))
}

func (r *BackendSuite) TestDeletesOlderStats(c *C) {
	ts := time.Now().Add(-scavengeTimeout)
	status := newStatus(anotherNode, ts)
	err := r.backend.UpdateNode(status)
	c.Assert(err, IsNil)

	err = r.backend.deleteOlderThan(ts.Add(time.Second))
	c.Assert(err, IsNil)

	var count int64
	when := timestamp(pb.TimeToProto(ts))
	err = r.backend.QueryRow(`SELECT COUNT(*) FROM probe WHERE captured_at = ?`, when).Scan(&count)
	c.Assert(err, IsNil)

	c.Assert(count, Equals, int64(0))
}

func (r *BackendSuite) TestGetsRecentStats(c *C) {
	clock := clockwork.NewFakeClock()

	err := addStatsForNode(r.backend, node, clock)
	c.Assert(err, IsNil)

	err = addStatsForNode(r.backend, anotherNode, clock)
	c.Assert(err, IsNil)

	status, err := r.backend.RecentStatus(node)
	c.Assert(err, IsNil)

	c.Assert(len(status.Probes), Equals, 5)
}

func (r *BackendWithClockSuite) TestScavengesOlderStats(c *C) {
	err := addStatsForNode(r.backend, node, r.clock)
	c.Assert(err, IsNil)

	r.clock.BlockUntil(1)
	r.clock.Advance(scavengeTimeout + time.Second)
	// block until the scavenge loop goes on another wait round
	r.clock.BlockUntil(1)
	status, err := r.backend.RecentStatus(node)
	c.Assert(err, IsNil)

	c.Assert(len(status.Probes), Equals, 0)
}

func addStatsForNode(b *backend, node string, clock clockwork.Clock) error {
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
