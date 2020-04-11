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

// NewCandidate returns a new election candidate.
// Specifid context is only used to initialize a session.
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
	case <-ctx.Done():
	}
}

// LeaderChan returns the channel that relays leadership changes.
// The returned channel needs to be serviced to avoid blocking the candidate
func (r *Candidate) LeaderChan() <-chan string {
	return r.leaderChan
}

// Pause pauses/resumes this candidate's campaign
func (r *Candidate) Pause(ctx context.Context, paused bool) {
	select {
	case r.pauseChan <- paused:
	case <-ctx.Done():
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
	var (
		ticker         clockwork.Ticker
		tickerC        <-chan time.Time
		sessionDone    <-chan struct{}
		observeChan    <-chan etcd.GetResponse
		errChan        <-chan error
		wg             sync.WaitGroup
		paused         bool
		election       *concurrency.Election
		session        *concurrency.Session
		campaignCtx    context.Context
		campaignCancel context.CancelFunc
		stopCampaign   = func() {
			campaignCancel()
			wg.Wait()
			errChan = nil
		}
		startCampaign = func() {
			if paused || errChan != nil {
				return
			}
			campaignCtx, campaignCancel = context.WithCancel(r.ctx)
			errChan = r.startCampaign(campaignCtx, election, &wg)
		}
		observeCtx    context.Context
		observeCancel context.CancelFunc
		stopObserve   = func() {
			observeCancel()
			observeChan = nil
		}
		startObserve = func() {
			if observeChan != nil {
				return
			}
			observeCtx, observeCancel = context.WithCancel(r.ctx)
			observeChan = election.Observe(observeCtx)
		}
		reconnect = func() error {
			var err error
			election, session, err = r.config.newElection(r.ctx, 0)
			if err != nil {
				return err
			}
			node, err := election.Leader(r.ctx)
			if err != nil && !isLeaderNotFoundError(err) {
				return err
			}
			sessionDone = nil
			if r.config.isLeader(node) {
				session.Orphan()
				election, session, err = r.config.resumeLeadership(r.ctx, node)
				if err != nil {
					return err
				}
			} else {
				// TODO(dmitri): this should not be the default, it starts the follower timer
				// and checks the leader. Should also start campaign on errors?
				startCampaign()
			}
			sessionDone = session.Done()
			startObserve()
			return nil
		}
	)
	defer func() {
		if ticker != nil {
			ticker.Stop()
		}
		if session != nil {
			ctx, cancel := context.WithTimeout(context.Background(), resignTimeout)
			r.resign(ctx, election, session)
			cancel()
			session.Close()
		}
		stopCampaign()
		stopObserve()
		close(r.leaderChan)
	}()

	campaignCtx, campaignCancel = context.WithCancel(r.ctx)
	observeCtx, observeCancel = context.WithCancel(r.ctx)
	var err error
	election, session, err = r.config.newElection(r.ctx, 0)
	if err == nil {
		// Start as a follower
		observeChan = election.Observe(observeCtx)
		sessionDone = session.Done()
		// TODO(dmitri): use another time to track follower state?
		ticker = r.config.clock.NewTicker(newRandomTimeout())
		tickerC = ticker.Chan()
	}

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
			if ok {
				r.config.WithField("leader", string(resp.Kvs[0].Value)).Debug("New leader.")
				// Leadership change event
				r.setLeader(string(resp.Kvs[0].Value))
				// TODO(dmitri): restart follower timer if not leader
				continue
			}
			r.config.Info("Observer chan closed unexpectedly.")
			stopObserve()
			if tickerC == nil {
				ticker = r.config.clock.NewTicker(r.config.ReconnectTimeout)
				tickerC = ticker.Chan()
			}
			// New state: reconnect after implicit error

		case err = <-errChan:
			stopCampaign()
			if err == nil {
				r.config.Debug("Elected successfully.")
				// New state: leader
				// Session expiration leads to another Campaign call
				continue
			}
			if !isNotLeaderError(err) {
				stopObserve()
				continue // New state: reconnect after error
			}
			err = nil
			r.config.WithError(err).Debug("Lost election.")

		case <-sessionDone:
			// TODO(dmitri): should we check the leader here as well and start campaign
			// (switch to candidate mode)?
			r.config.Debug("Session expired.")
			stopCampaign()
			stopObserve()
			if err = reconnect(); err == nil {
				if ticker != nil {
					ticker.Stop()
				}
				tickerC = nil
			}

		case <-tickerC:
			if err != nil {
				r.config.WithError(err).Warn("Candidate failed, will reconnect.")
			}
			if session != nil {
				session.Orphan()
			}
			if err = reconnect(); err == nil {
				ticker.Stop()
				tickerC = nil
			}

		case paused = <-r.pauseChan:
			logger := r.config.WithField("paused", paused)
			if paused {
				logger.Debug("Pausing campaign.")
				stopCampaign()
			} else {
				logger.Debug("Resuming campaign.")
				startCampaign()
			}

		case <-r.resignChan:
			if r.resign(r.ctx, election, session) == nil {
				stopCampaign()
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

func (r *Candidate) startCampaign(ctx context.Context, election *concurrency.Election, wg *sync.WaitGroup) <-chan error {
	// Buffered since errChan might not get a read before candidate is canceled
	errChan := make(chan error, 1)
	wg.Add(1)
	go func() {
		errChan <- election.Campaign(ctx, r.config.Name)
		wg.Done()
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

func isNotLeaderError(err error) bool {
	return err == concurrency.ErrElectionNotLeader
}

func isLeaderNotFoundError(err error) bool {
	return err == concurrency.ErrElectionNoLeader
}

func isContextCanceledError(err error) bool {
	return errors.Cause(trace.Unwrap(err)) == context.Canceled
}

const (
	defaultTerm = 60 * time.Second

	// TODO: make reconnects use exponential backoff
	defaultReconnectTimeout = 5 * time.Second

	resignTimeout = 5 * time.Second
)
