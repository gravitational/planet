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
		pauseChan:  make(chan bool),
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
	pauseChan  chan bool
}

// Stop stops the candidate's internal processes and releases resources
func (r *Candidate) Stop() {
	r.cancel()
	for range r.leaderChan {
	}
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

// Pause pauses/resumes this candidates campaign
func (r *Candidate) Pause(paused bool) {
	select {
	case r.pauseChan <- paused:
	case <-r.ctx.Done():
	}
}

// CandidateConfig defines candidate configuration
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
	// FieldLogger specifies the logger
	logrus.FieldLogger
	clock clockwork.Clock
}

func (r *Candidate) loop() {
	defer close(r.leaderChan)

	var ticker clockwork.Ticker
	defer func() {
		if ticker != nil {
			ticker.Stop()
		}
	}()
	var tickerC <-chan time.Time
	var sessionDone <-chan struct{}
	var observeChan <-chan etcd.GetResponse
	var errChan <-chan error
	ctx, cancel := context.WithCancel(r.ctx)
	election, session, err := r.config.newElection(r.ctx, 0)
	defer func() {
		if session != nil {
			ctx, cancel := context.WithTimeout(context.Background(), resignTimeout)
			r.resign(ctx, election, session)
			cancel()
			session.Close()
		}
		if cancel != nil {
			cancel()
		}
		r.wg.Wait()
	}()
	if err == nil {
		// Start as a candidate
		errChan = r.startCampaign(ctx, election)
		observeChan = election.Observe(r.ctx)
		sessionDone = session.Done()
	}
	var paused bool

	for {
		select {
		case <-r.ctx.Done():
			// nolint:lostcancel
			return
		default:
		}
		if err != nil && tickerC == nil {
			ticker = r.config.clock.NewTicker(r.config.ReconnectTimeout)
			tickerC = ticker.Chan()
		}
		select {
		case resp, ok := <-observeChan:
			if !ok {
				r.config.Info("Observer chan closed unexpectedly.")
				observeChan = nil
				if tickerC == nil {
					ticker = r.config.clock.NewTicker(r.config.ReconnectTimeout)
					tickerC = ticker.Chan()
				}
				continue
			}
			r.config.WithField("leader", string(resp.Kvs[0].Value)).Debug("New leader.")
			// Leadership change event
			r.setLeader(string(resp.Kvs[0].Value))

		case err = <-errChan:
			cancel()
			r.wg.Wait()
			errChan = nil
			if err == nil {
				r.config.Debug("Elected successfully.")
				continue
			}
			session.Close()
			session = nil
			sessionDone = nil

		case <-sessionDone:
			r.config.Debug("Session expired.")
			election, session, err = r.config.newElection(r.ctx, 0)
			if err == nil {
				sessionDone = session.Done()
			}

		case <-tickerC:
			r.config.WithError(err).Warn("Candidate failed, will reconnect.")
			if session != nil {
				session.Orphan()
			}
			election, session, err = r.config.newElection(r.ctx, 0)
			if err != nil {
				continue
			}
			sessionDone = session.Done()
			if observeChan == nil {
				observeChan = election.Observe(r.ctx)
			}
			node, err := election.Leader(r.ctx)
			if err != nil && !isLeaderNotFoundError(err) {
				continue
			}
			if r.config.isLeader(node) {
				session.Orphan()
				election, session, err = r.config.resumeLeadership(r.ctx, node)
				if err != nil {
					continue
				}
				sessionDone = session.Done()
			} else if !paused && errChan == nil {
				ctx, cancel = context.WithCancel(r.ctx)
				errChan = r.startCampaign(ctx, election)
			}
			ticker.Stop()
			tickerC = nil

		case paused = <-r.pauseChan:
			logger := r.config.WithField("paused", paused)
			if paused {
				logger.Debug("Pausing campaign.")
				cancel()
				r.wg.Wait()
				errChan = nil
			} else if errChan == nil {
				logger.Debug("Resuming campaign.")
				// nolint:lostcancel
				ctx, cancel = context.WithCancel(r.ctx)
				errChan = r.startCampaign(ctx, election)
			}

		case <-r.resignChan:
			if r.resign(r.ctx, election, session) == nil {
				select {
				case <-r.config.clock.After(2 * r.config.Term):
				case <-r.ctx.Done():
				}
			}

		case <-r.ctx.Done():
			return
		}
	}
}

func (r *Candidate) startCampaign(ctx context.Context, election *concurrency.Election) <-chan error {
	// Buffered since errChan might not get a read before candidate is canceled
	errChan := make(chan error, 1)
	r.wg.Add(1)
	go func() {
		errChan <- election.Campaign(ctx, r.config.Name)
		r.wg.Done()
	}()
	return errChan
}

func (r *Candidate) resign(ctx context.Context, election *concurrency.Election, session *concurrency.Session) error {
	node, err := election.Leader(ctx)
	if err != nil {
		return trace.Wrap(err)
	}
	if !r.config.isLeader(node) {
		return nil
	}
	r.config.Info("Step down.")
	election = concurrency.ResumeElection(session, r.config.Prefix,
		string(node.Kvs[0].Key), node.Kvs[0].CreateRevision)
	return election.Resign(ctx)
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

func (r *CandidateConfig) isLeader(node *etcd.GetResponse) bool {
	return node != nil && len(node.Kvs) != 0 && r.Name == string(node.Kvs[0].Value)
}

func (r *CandidateConfig) termSeconds() int {
	return int(r.Term.Truncate(time.Second).Seconds())
}

func (r *CandidateConfig) termContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), r.Term)
}

func (r *CandidateConfig) resumeLeadership(ctx context.Context, node *etcd.GetResponse) (*concurrency.Election, *concurrency.Session, error) {
	session, err := r.newSession(ctx, etcd.LeaseID(node.Kvs[0].Lease))
	if err != nil {
		return nil, nil, trace.Wrap(err)
	}
	return concurrency.ResumeElection(session, r.Prefix, string(node.Kvs[0].Key), node.Kvs[0].CreateRevision), session, nil
}

func (r *CandidateConfig) newElection(ctx context.Context, leaseID etcd.LeaseID) (*concurrency.Election, *concurrency.Session, error) {
	session, err := r.newSession(ctx, leaseID)
	if err != nil {
		return nil, nil, trace.Wrap(err)
	}
	return concurrency.NewElection(session, r.Prefix), session, nil
}

func (r *CandidateConfig) newSession(ctx context.Context, leaseID etcd.LeaseID) (*concurrency.Session, error) {
	// leaseID has preference if specified
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
	defaultTerm = 60 * time.Second

	// TODO: make reconnects use exponential backoff
	defaultReconnectTimeout = 5 * time.Second

	resignTimeout = 1 * time.Second
)
