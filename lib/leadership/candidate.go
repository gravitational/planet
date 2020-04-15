package leadership

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/gravitational/trace"
	"github.com/jonboulle/clockwork"
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
	return &Candidate{
		config:     config,
		ctx:        ctx,
		cancel:     cancel,
		resignChan: make(chan struct{}),
		leaderChan: make(chan string),
		pauseChan:  make(chan bool),
	}, nil
}

type Candidate struct {
	config     CandidateConfig
	ctx        context.Context
	cancel     context.CancelFunc
	resignChan chan struct{}
	leaderChan chan string
	pauseChan  chan bool
}

// Start starts this candidate's processes
func (r *Candidate) Start() {
	go r.loop(true)
}

// StartObserver starts this candidate's processes as an observer
func (r *Candidate) StartObserver() {
	go r.loop(false)
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
// The channel will receive the name of the active leader (i.e. as configured with CandidateConfig.Name).
// The returned channel needs to be serviced to avoid blocking the internal process
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

func (r *Candidate) loop(enabled bool) {
	defer close(r.leaderChan)
	election, session, err := r.config.newElection(r.ctx, 0)
	defer func() {
		if election != nil {
			ctx, cancel := context.WithTimeout(context.Background(), resignTimeout)
			election.Resign(ctx)
			cancel()
		}
	}()
	if err == nil {
		// Perform initial work
		if err := r.work(election, session, !enabled); err != nil && trace.Unwrap(err) == context.Canceled {
			return
		}
	}
	ticker := r.config.clock.NewTicker(r.config.ReconnectTimeout)
	tickerC := ticker.Chan()
	for {
		select {
		case <-tickerC:
			// TODO(dmitri): resume election with the previous value before trying to aquire a new lease?
			// For this to work, the session needs to be handled here and not reset in r.work
			election, session, err = r.config.newElection(r.ctx, 0)
			if err != nil {
				r.config.WithError(err).Warn("Unable to create new session.")
				continue
			}
			ticker.Stop()
			tickerC = nil
			if err := r.work(election, session, !enabled); err != nil {
				r.config.WithError(err).Warn("Candidate failed, will reconnect.")
				ticker = r.config.clock.NewTicker(r.config.ReconnectTimeout)
				tickerC = ticker.Chan()
			}

		case <-r.ctx.Done():
			return
		}
	}
}

// work is the main process of the Candidate and is blocking.
// Candidate starts as a follower and after a random election timeout, starts its own campaign.
// The campaign either ends with it becoming a leader, or it transitions to follower again and the cycle
// starts from scratch.
//
// If there are any errors encountered during the campaign or the session expires or the watcher chan
// is closed unexpectedly, the method will return with a corresponding error.
func (r *Candidate) work(election *concurrency.Election, session *concurrency.Session, paused bool) (err error) {
	campaignCtx, campaignCancel := context.WithCancel(r.ctx)
	observeCtx, observeCancel := context.WithCancel(r.ctx)
	var wg sync.WaitGroup
	defer func() {
		observeCancel()
		campaignCancel()
		wg.Wait()
		session.Orphan()
	}()
	var errChan <-chan error
	observeChan := election.Observe(observeCtx)
	var timerC <-chan time.Time
	if !paused {
		// Start as a follower
		timerC = r.config.clock.After(electionTimeout())
	}
	for {
		select {
		case resp, ok := <-observeChan:
			if !ok {
				observeCancel()
				return observerClosedError
			}
			leader := string(resp.Kvs[0].Value)
			r.config.WithField("leader", leader).Debug("New leader.")
			r.setLeader(leader)
			if leader != r.config.Name && errChan == nil && timerC == nil && !paused {
				// Trigger election explicitly
				timerC = r.config.clock.After(electionTimeout())
			}

		case <-timerC:
			// Transition to candidate: start campaign
			timerC = nil
			errChan = r.startCampaign(campaignCtx, election, &wg)

		case err = <-errChan:
			errChan = nil
			campaignCancel()
			wg.Wait()
			if err == nil {
				r.config.Debug("Elected successfully.")
				continue
			}
			if !isNotLeaderError(err) {
				return err
			}
			r.config.Debug("Lost election.")
			timerC = r.config.clock.After(electionTimeout())

		case <-session.Done():
			r.config.Warn("Session expired.")
			return sessionExpiredError

		case paused = <-r.pauseChan:
			logger := r.config.WithField("paused", paused)
			if !paused {
				logger.Debug("Resume.")
				if errChan == nil && timerC == nil {
					timerC = r.config.clock.After(electionTimeout())
				}
				continue
			}
			logger.Debug("Pause.")
			timerC = nil
			if errChan != nil {
				campaignCancel()
				wg.Wait()
				errChan = nil
			}
			election.Resign(r.ctx)

		case <-r.resignChan:
			if errChan != nil {
				campaignCancel()
				wg.Wait()
				errChan = nil
			}
			election.Resign(r.ctx)

		case <-r.ctx.Done():
			return r.ctx.Err()
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

func (r *Candidate) setLeader(leader string) {
	select {
	case r.leaderChan <- leader:
	case <-r.ctx.Done():
	}
}

func (r *CandidateConfig) checkAndSetDefaults() error {
	if r.Term == 0 {
		r.Term = defaultTerm
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

// electionTimeout generates a random timeout for a candidate's election.
// Its prupose is to allow some room in the elections by avoiding all candidates
// from starting the elections at the time
func electionTimeout() time.Duration {
	return time.Duration(150+rand.Intn(150)) * time.Millisecond
}

const (
	defaultTerm             = 60 * time.Second
	defaultReconnectTimeout = 5 * time.Second
	resignTimeout           = 5 * time.Second
)

var (
	observerClosedError = trace.Errorf("observer closed unexpectedly")
	sessionExpiredError = trace.Errorf("session expired")
)
