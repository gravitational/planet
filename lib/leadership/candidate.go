package leadership

import (
	"context"
	"sync"
	"time"

	"github.com/gravitational/trace"
	"github.com/jonboulle/clockwork"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	etcd "go.etcd.io/etcd/clientv3"
	"go.etcd.io/etcd/clientv3/concurrency"
	"go.etcd.io/etcd/mvcc/mvccpb"
)

// NewCandidate returns a new election candidates.
// Specifid context is used to initialize a session
func NewCandidate(ctx context.Context, config CandidateConfig) (*Candidate, error) {
	if err := config.checkAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	candidate := &Candidate{
		config:  config,
		ctx:     ctx,
		cancel:  cancel,
		resignC: make(chan struct{}),
		leaderC: make(chan string),
	}
	go candidate.loop()
	return candidate, nil
}

type Candidate struct {
	config CandidateConfig
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	resignC chan struct{}
	leaderC chan string
}

// Stop stops the candidate's internal processes and releases resources
func (r *Candidate) Stop() {
	r.cancel()
	r.wg.Wait()
}

// StepDown gives up leadership
func (r *Candidate) StepDown(ctx context.Context) {
	select {
	case r.resignC <- struct{}{}:
	case <-r.ctx.Done():
	}
}

// LeaderChan returns the channel that updates leader changes
func (r *Candidate) LeaderChan() <-chan string {
	return r.leaderC
}

type CandidateConfig struct {
	// Term specifies the duration of an election term.
	// Defaults to defaultTerm if unspecified
	Term time.Duration
	// Prefix specifies the key prefix to use for the elections
	Prefix string
	// Name specifies this candidate's name
	Name string
	// Client specifies the etcd client
	Client *etcd.Client
	// Timeout specifies the timeout for reconnects
	ReconnectTimeout time.Duration
	logrus.FieldLogger
	clock clockwork.Clock
}

func (r *Candidate) loop() {
	var election *concurrency.Election
	var session *concurrency.Session
	var ticker clockwork.Ticker
	var tickerC <-chan time.Time
	var err error

	election, session, err = r.config.newElection(r.ctx, 0)
	for {
		if err != nil {
			if ticker == nil {
				ticker = r.config.clock.NewTicker(r.config.ReconnectTimeout)
				tickerC = ticker.Chan()
			}
		} else {
			if ticker != nil {
				ticker.Stop()
				tickerC = nil
			}
		}

		select {
		case <-r.ctx.Done():
			if session != nil {
				session.Close()
			}
			return

		case <-tickerC:
			election, session, err = r.config.newElection(r.ctx, 0)
			if err == nil {
				ticker.Stop()
				tickerC = nil
			}
			r.config.WithError(err).Warn("Failed to reconnect.")
			continue

		case <-r.resignC:
			if election != nil {
				if err := r.resign(election, session); err != nil {
					r.config.WithError(err).Warn("Failed to step down.")
					continue
				}
			}
		}

		node, err := election.Leader(r.ctx)
		if err != nil && !isLeaderNotFoundError(err) {
			r.config.WithError(err).Warn("Failed to query leader.")
			continue
		}

		if node != nil {
			leader := *node.Kvs[0]
			if r.config.isLeader(leader) {
				election, session, err = r.config.resumeLeadership(r.ctx, leader)
				if err != nil {
					r.config.WithError(err).Warn("Failed to resume leadership.")
					continue
				}
				err = r.observe(election, session)
				if err != nil {
					r.config.WithError(err).Warn("Failed to observe elections.")
				}
				continue
			}
		}

		err = r.elect(election, session)
		if err != nil {
			r.config.WithError(err).Warn("Failed to elect.")
		}
	}
}

func (r *Candidate) elect(election *concurrency.Election, session *concurrency.Session) error {
	errChan := make(chan error)
	go func() {
		errChan <- election.Campaign(r.ctx, r.config.Prefix)
	}()

	select {
	case err := <-errChan:
		if err != nil {
			if isContextCanceledError(err) {
				return trace.Wrap(err)
			}
			session.Close()
		}
		return trace.Wrap(err)

	case <-r.ctx.Done():
		session.Close()
		return trace.Wrap(r.ctx.Err())

	case <-session.Done():
		return trace.LimitExceeded("session expired")
	}
}

func (r *Candidate) observe(election *concurrency.Election, session *concurrency.Session) error {
	observeChan := election.Observe(r.ctx)
	for {
		select {
		case resp, ok := <-observeChan:
			if !ok {
				session.Close()
				return trace.BadParameter("TODO")
			}
			r.setLeader(string(resp.Kvs[0].Value))

		case <-r.resignC:
			return r.resign(election, session)

		case <-r.ctx.Done():
			r.resign(election, session)
			return trace.Wrap(r.ctx.Err())

		case <-session.Done():
			return trace.LimitExceeded("session expired")
		}
	}
}

func (r *Candidate) resign(election *concurrency.Election, session *concurrency.Session) error {
	node, err := election.Leader(r.ctx)
	if err != nil {
		return trace.Wrap(err)
	}
	leader := *node.Kvs[0]
	if !r.config.isLeader(leader) {
		return nil
	}
	election = concurrency.ResumeElection(session, r.config.Prefix,
		string(leader.Key), leader.CreateRevision)
	return election.Resign(r.ctx)
}

func (r *Candidate) setLeader(leader string) {
	select {
	case r.leaderC <- leader:
	case <-r.ctx.Done():
	}
}

func (r *CandidateConfig) checkAndSetDefaults() error {
	if r.Term == 0 {
		r.Term = defaultTerm
	}
	if r.FieldLogger == nil {
		r.FieldLogger = logrus.WithField(trace.Component, "candidate")
	}
	if r.ReconnectTimeout == 0 {
		r.ReconnectTimeout = defaultReconnectTimeout
	}
	if r.Term < time.Second {
		return trace.BadParameter("election term cannot be less than a second")
	}
	if r.Prefix == "" {
		return trace.BadParameter("election prefix cannot be empty")
	}
	if r.Name == "" {
		return trace.BadParameter("candidate name cannot be empty")
	}
	if r.Client == nil {
		return trace.BadParameter("etcd client cannot be empty")
	}
	return nil
}

func (r *CandidateConfig) isLeader(node mvccpb.KeyValue) bool {
	return r.Name == string(node.Value)
}

func (r *CandidateConfig) termSeconds() int {
	return int(r.Term.Truncate(time.Second).Seconds())
}

func (r *CandidateConfig) termContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), r.Term)
}

func (r *CandidateConfig) resumeLeadership(ctx context.Context, leader mvccpb.KeyValue) (*concurrency.Election, *concurrency.Session, error) {
	session, err := r.newSession(ctx, etcd.LeaseID(leader.Lease))
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return concurrency.ResumeElection(session, r.Prefix, string(leader.Key), leader.CreateRevision), session, nil
}

func (r *CandidateConfig) newElection(ctx context.Context, leaseID etcd.LeaseID) (*concurrency.Election, *concurrency.Session, error) {
	session, err := r.newSession(ctx, leaseID)
	if err != nil {
		return nil, nil, trace.Wrap(err)
	}
	return concurrency.NewElection(session, r.Prefix), session, nil
}

func (r *CandidateConfig) newSession(ctx context.Context, leaseID etcd.LeaseID) (*concurrency.Session, error) {
	return concurrency.NewSession(r.Client,
		concurrency.WithContext(ctx),
		concurrency.WithTTL(r.termSeconds()),
		concurrency.WithLease(leaseID),
	)
}

func isLeaderNotFoundError(err error) bool {
	return trace.Unwrap(err) == concurrency.ErrElectionNoLeader
}

func isContextCanceledError(err error) bool {
	return errors.Cause(trace.Unwrap(err)) == context.Canceled
}

const (
	defaultTerm = 10 * time.Second
	// TODO: make reconnects use exponential backoff
	defaultReconnectTimeout = 5 * time.Second
)
