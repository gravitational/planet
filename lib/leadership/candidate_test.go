package leadership

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gravitational/trace"
	"github.com/sirupsen/logrus"
	etcd "go.etcd.io/etcd/clientv3"
	"google.golang.org/grpc"
	. "gopkg.in/check.v1"
)

func TestLeadership(t *testing.T) { TestingT(t) }

type S struct {
	client *etcd.Client
}

var _ = Suite(&S{})

func (s *S) SetUpSuite(c *C) {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetOutput(os.Stderr)
	endpointsEnv := os.Getenv("PLANET_TEST_ETCD_ENDPOINTS")
	if endpointsEnv == "" {
		// Skip the suite
		c.Skip("This test requires etcd, specify endpoint(s) as a comma-separated list in PLANET_TEST_ETCD_ENDPOINTS")
		return
	}
	config := config{
		endpoints:               strings.Split(endpointsEnv, ","),
		headerTimeoutPerRequest: 1 * time.Second,
	}
	config.setDefaults()
	var err error
	s.client, err = config.newClient()
	c.Assert(err, IsNil)
}

func (s *S) TestCanStopCandidate(c *C) {
	candidate, err := NewCandidate(context.TODO(), CandidateConfig{
		Term:   1 * time.Second,
		Prefix: "testleader",
		Name:   "candidate",
		Client: s.client,
	})
	c.Assert(err, IsNil)
	candidate.Stop()
}

func (s *S) TestCandidateCanElectItself(c *C) {
	candidate, err := NewCandidate(context.TODO(), CandidateConfig{
		Term:   1 * time.Second,
		Prefix: "testleader",
		Name:   "candidate",
		Client: s.client,
	})
	c.Assert(err, IsNil)
	defer candidate.Stop()
	c.Assert(err, IsNil)
	c.Assert(<-candidate.LeaderChan(), Equals, "candidate")
}

func (s *S) TestCandidateCanStepDown(c *C) {
	const term = 1 * time.Second
	candidate1, err := NewCandidate(context.TODO(), CandidateConfig{
		Term:   term,
		Prefix: "testleader",
		Name:   "candidate1",
		Client: s.client,
	})
	c.Assert(err, IsNil)
	defer candidate1.Stop()
	candidate2, err := newCandidate(context.TODO(), CandidateConfig{
		Term:   term,
		Prefix: "testleader",
		Name:   "candidate2",
		Client: s.client,
	})
	c.Assert(err, IsNil)
	defer candidate2.Stop()

	respChan := make(chan string, 2)
	go drainChan(candidate2.LeaderChan())
	go func() {
		for resp := range candidate1.LeaderChan() {
			respChan <- resp
		}
	}()

	c.Assert(<-respChan, Equals, "candidate1")

	candidate2.start()
	candidate1.StepDown(context.TODO())

	c.Assert(<-respChan, Equals, "candidate2")
}

func (s *S) TestCandidateCanPause(c *C) {
	const term = 1 * time.Second
	candidate1, err := NewCandidate(context.TODO(), CandidateConfig{
		Term:   term,
		Prefix: "testleader",
		Name:   "candidate1",
		Client: s.client,
	})
	c.Assert(err, IsNil)
	defer candidate1.Stop()
	candidate2, err := newCandidate(context.TODO(), CandidateConfig{
		Term:   term,
		Prefix: "testleader",
		Name:   "candidate2",
		Client: s.client,
	})
	c.Assert(err, IsNil)
	defer candidate2.Stop()

	respChan := make(chan string, 2)
	go drainChan(candidate2.LeaderChan())
	go func() {
		for resp := range candidate1.LeaderChan() {
			respChan <- resp
		}
	}()

	c.Assert(<-respChan, Equals, "candidate1")

	candidate2.start()
	candidate1.Pause(context.TODO(), true)

	c.Assert(<-respChan, Equals, "candidate2")
}

// newClient creates a new instance of an etcdv3 client
func (r *config) newClient() (*etcd.Client, error) {
	client, err := etcd.New(etcd.Config{
		Endpoints:          r.endpoints,
		DialTimeout:        r.dialTimeout,
		DialOptions:        []grpc.DialOption{grpc.WithBlock()},
		MaxCallSendMsgSize: clientMaxCallSendMsgSize,
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return client, nil
}

type config struct {
	endpoints               []string
	dialTimeout             time.Duration
	headerTimeoutPerRequest time.Duration
}

func (r *config) setDefaults() {
	if r.dialTimeout == 0 {
		r.dialTimeout = 5 * time.Second
	}
	if r.headerTimeoutPerRequest == 0 {
		r.headerTimeoutPerRequest = 1 * time.Second
	}
}

func newCandidate(ctx context.Context, config CandidateConfig) (*Candidate, error) {
	if err := config.checkAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	candidate := &Candidate{
		config:     config,
		ctx:        ctx,
		cancel:     cancel,
		resignChan: make(chan struct{}),
		leaderChan: make(chan string),
	}
	return candidate, nil
}

func (r *Candidate) start() {
	go r.loop()
}

func drainChan(ch <-chan string) {
	for range ch {
	}
}

const (
	clientMaxCallSendMsgSize = 100 * 1024 * 1024
)
