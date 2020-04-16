package leadership

import (
	"context"
	"sync"
	"time"

	"github.com/gravitational/trace"
	"github.com/jonboulle/clockwork"
	"github.com/sirupsen/logrus"
	etcd "go.etcd.io/etcd/clientv3"
	v3 "go.etcd.io/etcd/clientv3"
	"go.etcd.io/etcd/clientv3/concurrency"
	pb "go.etcd.io/etcd/etcdserver/etcdserverpb"
	"go.etcd.io/etcd/mvcc/mvccpb"
)

// NewWatcher returns a new watcher to relay changes to the specified key.
// The watcher is automatically active.
func NewWatcher(config WatcherConfig) (*Watcher, error) {
	if err := config.checkAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	w := &Watcher{
		config:   config,
		ctx:      ctx,
		cancel:   cancel,
		respChan: make(chan v3.Event),
	}
	return w, nil
}

// Watcher can follow and push notifications whenever
// there is a change for the specified key.
type Watcher struct {
	config   WatcherConfig
	respChan chan v3.Event
	ctx      context.Context
	cancel   context.CancelFunc
}

// WatcherConfig defines watcher configuration
type WatcherConfig struct {
	// Key specifies the key to watch
	Key string
	// Client specifies the etcd client
	Client *v3.Client
	// RetryTimeout specifies the timeout between reconnect attempts
	RetryTimeout time.Duration
	// FieldLogger specifies the logger
	logrus.FieldLogger
	clock clockwork.Clock
}

// Start starts the watcher's internal process
func (r *Watcher) Start() {
	go r.loop()
}

// Stop requests watcher to stop and release resources
func (r *Watcher) Stop() {
	r.cancel()
	for range r.respChan {
	}
}

// RespChan returns the channel that the watcher is relaying the updates on.
// The channel must be served to avoid blocking the watcher
func (r *Watcher) RespChan() <-chan v3.Event {
	return r.respChan
}

// loop runs the top-level watcher process.
// This is a blocking method as the watcher will automatically attempt reconnects
// on any errors.
func (r *Watcher) loop() {
	defer close(r.respChan)

	var ticker clockwork.Ticker
	var tickerC <-chan time.Time
	var wg sync.WaitGroup
	var wc *watcher
	var observeChan <-chan v3.GetResponse
	var sessionDone <-chan struct{}
	ctx, cancel := context.WithCancel(r.ctx)
	session, err := r.config.newSession(r.ctx, 0)
	defer func() {
		cancel()
		wg.Wait()
		if session != nil {
			session.Orphan()
		}
	}()
	if err == nil {
		wc = r.config.newWatcher(session)
		observeChan = wc.observe(ctx, &wg)
		sessionDone = session.Done()
	} else {
		ticker = r.config.newTicker()
		tickerC = ticker.Chan()
	}

	for {
		select {
		case resp, ok := <-observeChan:
			if !ok {
				r.config.Warn("Observer chan closed unexpectedly.")
				cancel()
				wg.Wait()
				observeChan = nil
				session.Orphan()
				sessionDone = nil
				if ticker == nil {
					ticker = r.config.newTicker()
					tickerC = ticker.Chan()
				}
				continue
			}
			// Update the receiver
			select {
			case r.respChan <- v3.Event{
				Kv: resp.Kvs[0],
			}:
			case <-r.ctx.Done():
				return
			}

		case <-tickerC:
			session, err = r.config.newSession(r.ctx, 0)
			if err != nil {
				r.config.WithError(err).Warn("Failed to create new session.")
				continue
			}
			wc = r.config.newWatcher(session)
			observeChan = wc.observe(ctx, &wg)
			sessionDone = session.Done()
			ticker.Stop()
			tickerC = nil

		case <-sessionDone:
			r.config.Warn("Session expired.")
			cancel()
			wg.Wait()
			observeChan = nil
			sessionDone = nil
			if ticker == nil {
				ticker = r.config.newTicker()
				tickerC = ticker.Chan()
			}

		case <-r.ctx.Done():
			return
		}
	}
}

type watcher struct {
	session *concurrency.Session
	key     string
}

func (r *watcher) observe(ctx context.Context, wg *sync.WaitGroup) <-chan v3.GetResponse {
	ch := make(chan v3.GetResponse)
	wg.Add(1)
	go func() {
		r.loop(ctx, ch)
		wg.Done()
	}()
	return ch
}

// observe reliably observes ordered changes on the underlying Key
// as GetResponse values. It will not necessarily fetch all historical key updates,
// but will always post the most recent key value.
//
// The channel closes when the context is canceled or the underlying watcher
// is otherwise disrupted.
// Adopted from https://github.com/etcd-io/etcd/blob/v3.4.3/clientv3/concurrency/election.go#L173
func (r *watcher) loop(ctx context.Context, ch chan<- v3.GetResponse) {
	client := r.session.Client()

	defer close(ch)
	for {
		resp, err := client.Get(ctx, r.key, v3.WithFirstCreate()...)
		if err != nil {
			return
		}

		var kv *mvccpb.KeyValue
		var hdr *pb.ResponseHeader

		if len(resp.Kvs) == 0 {
			cctx, cancel := context.WithCancel(ctx)
			// wait for first key put on prefix
			opts := []v3.OpOption{v3.WithRev(resp.Header.Revision), v3.WithPrefix()}
			wch := client.Watch(cctx, r.key, opts...)
			for kv == nil {
				wr, ok := <-wch
				if !ok || wr.Err() != nil {
					cancel()
					return
				}
				// only accept puts; a delete will make observe() spin
				for _, ev := range wr.Events {
					if ev.Type == mvccpb.PUT {
						hdr, kv = &wr.Header, ev.Kv
						// may have multiple revs; hdr.rev = the last rev
						// set to kv's rev in case batch has multiple Puts
						hdr.Revision = kv.ModRevision
						break
					}
				}
			}
			cancel()
		} else {
			hdr, kv = resp.Header, resp.Kvs[0]
		}

		select {
		case ch <- v3.GetResponse{Header: hdr, Kvs: []*mvccpb.KeyValue{kv}}:
		case <-ctx.Done():
			return
		}

		cctx, cancel := context.WithCancel(ctx)
		wch := client.Watch(cctx, string(kv.Key), v3.WithRev(hdr.Revision+1))
		keyDeleted := false
		for !keyDeleted {
			wr, ok := <-wch
			if !ok {
				cancel()
				return
			}
			for _, ev := range wr.Events {
				if ev.Type == mvccpb.DELETE {
					keyDeleted = true
					break
				}
				resp.Header = &wr.Header
				resp.Kvs = []*mvccpb.KeyValue{ev.Kv}
				select {
				case ch <- *resp:
				case <-cctx.Done():
					cancel()
					return
				}
			}
		}
		cancel()
	}
}

func (r *WatcherConfig) checkAndSetDefaults() error {
	if r.Key == "" {
		return trace.BadParameter("key cannot be empty")
	}
	if r.Client == nil {
		return trace.BadParameter("client cannot be empty")
	}
	if r.RetryTimeout == 0 {
		r.RetryTimeout = retryTimeout
	}
	if r.FieldLogger == nil {
		r.FieldLogger = logrus.WithFields(logrus.Fields{
			trace.Component: "watcher",
			"key":           r.Key,
		})
	}
	if r.clock == nil {
		r.clock = clockwork.NewRealClock()
	}
	return nil
}

func (r *WatcherConfig) newWatcher(session *concurrency.Session) *watcher {
	return &watcher{
		session: session,
		key:     r.Key,
	}
}

func (r *WatcherConfig) newTicker() clockwork.Ticker {
	return r.clock.NewTicker(r.RetryTimeout)
}

func (r *WatcherConfig) newSession(ctx context.Context, leaseID etcd.LeaseID) (*concurrency.Session, error) {
	// leaseID has preference if specified
	return concurrency.NewSession(r.Client,
		concurrency.WithContext(ctx),
		concurrency.WithLease(leaseID),
	)
}

const retryTimeout = 5 * time.Second
