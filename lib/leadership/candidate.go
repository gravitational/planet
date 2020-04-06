package elections

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
// Specifid context is used to initialize a session.
// The returned candidate is automatically active
func NewCandidate(ctx context.Context, config CandidateConfig) (*Candidate, error) {
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
	go candidate.loop()
	return candidate, nil
}

type Candidate struct {
	config     CandidateConfig
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	resignChan chan struct{}
	leaderChan chan string
}

// Stop stops the candidate's internal processes and releases resources
func (r *Candidate) Stop() {
	r.cancel()
	r.wg.Wait()
}

// StepDown gives up leadership
func (r *Candidate) StepDown(ctx context.Context) {
	select {
	case r.resignChan <- struct{}{}:
	case <-r.ctx.Done():
	}
}

// LeaderChan returns the channel that relays leadership changes.
// The returned channel needs to be serviced to avoid blocking the candidate
func (r *Candidate) LeaderChan() <-chan string {
	return r.leaderChan
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
	defer close(r.leaderChan)

	var ticker clockwork.Ticker
	var tickerC <-chan time.Time
	election, session, err := r.config.newElection(r.ctx, 0)

	for {
		if err != nil {
			if tickerC == nil {
				ticker = r.config.clock.NewTicker(r.config.ReconnectTimeout)
				tickerC = ticker.Chan()
			}
			select {
			case <-r.ctx.Done():
				if session != nil {
					session.Close()
				}
				return

			case <-tickerC:
				r.config.WithError(err).Warn("Candidate failed.")
				if session != nil {
					// As per documentation, the session is orphaned
					// when the client state is indeterminate
					session.Orphan()
				}
				election, session, err = r.config.newElection(r.ctx, 0)
				if err != nil {
					continue
				}
				ticker.Stop()
				tickerC = nil

			case <-r.resignChan:
				if election != nil {
					err = r.resign(election, session)
				}
			}
		}

		node, err := election.Leader(r.ctx)
		r.config.WithField("node", node).WithError(err).Info("Query leader.")
		if err != nil && !isLeaderNotFoundError(err) {
			continue
		}
		if node != nil {
			leader := *node.Kvs[0]
			logger := r.config.WithField("leader", string(leader.Value))
			logger.Info("New leader.")
			if r.config.isLeader(leader) {
				logger.Info("We are still the leader, resume leadership.")
				election, session, err = r.config.resumeLeadership(r.ctx, leader)
				if err != nil {
					logger.WithError(err).Info("Failed to resume leadership.")
					continue
				}
				logger.Info("Start observing the elections.")
				err = r.observe(election, session)
				logger.WithError(err).Info("Ran observe loop.")
				continue
			}
		}
		err = r.elect(election, session)
		r.config.WithError(err).Info("Ran elect loop.")
	}
}

func (r *Candidate) elect(election *concurrency.Election, session *concurrency.Session) error {
	var wg sync.WaitGroup
	wg.Add(1)
	ctx, cancel := context.WithCancel(r.ctx)
	defer func() {
		cancel()
		wg.Wait()
	}()
	errChan := make(chan error)
	go func() {
		errChan <- election.Campaign(ctx, r.config.Name)
		wg.Done()
	}()

	for {
		select {
		case err := <-errChan:
			r.config.WithError(err).Info("Ran campaign.")
			if err != nil {
				return trace.Wrap(err)
			}
			r.config.Info("Me leader.")
			r.setLeader(r.config.Name)
			return nil

		case <-r.resignChan:
			// Not a leader, so simply consume the request

		case <-r.ctx.Done():
			return trace.Wrap(r.ctx.Err())

		case <-session.Done():
			return trace.LimitExceeded("session expired")
		}
	}
}

// TODO: each candidate needs to be running the observer loop to keep leader updates going!
// How does this play with leases?
func (r *Candidate) observe(election *concurrency.Election, session *concurrency.Session) error {
	ctx, cancel := context.WithCancel(r.ctx)
	defer cancel()
	// TODO: factor this to the top-level so this can be queried along with resign and r.ctx!!!
	observeChan := election.Observe(ctx)
	for {
		select {
		case resp, ok := <-observeChan:
			if !ok {
				return trace.BadParameter("observer closed with error")
			}
			r.config.WithField("leader", string(resp.Kvs[0].Value)).Info("Him leader.")
			r.setLeader(string(resp.Kvs[0].Value))
			if !r.config.isLeader(*resp.Kvs[0]) {
				// Attempt to re-elect
				return nil
			}

		case <-r.resignChan:
			// FIXME: this needs to be handled outside
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
	r.config.Info("Step down.")
	election = concurrency.ResumeElection(session, r.config.Prefix,
		string(leader.Key), leader.CreateRevision)
	return election.Resign(r.ctx)
}

func (r *Candidate) setLeader(leader string) {
	select {
	case r.leaderChan <- leader:
	case <-r.ctx.Done():
	}
}

func (r *CandidateConfig) checkAndSetDefaults() error {
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
	if r.Term == 0 {
		r.Term = defaultTerm
	}
	if r.FieldLogger == nil {
		r.FieldLogger = logrus.WithFields(logrus.Fields{
			trace.Component: "candidate",
			"id":            r.Name,
		})
	}
	if r.ReconnectTimeout == 0 {
		r.ReconnectTimeout = defaultReconnectTimeout
	}
	if r.clock == nil {
		r.clock = clockwork.NewRealClock()
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
		return nil, nil, trace.Wrap(err)
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
