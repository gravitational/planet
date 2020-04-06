package leadership

import (
	"context"
	"time"

	"github.com/gravitational/trace"
	"github.com/jonboulle/clockwork"
	"github.com/sirupsen/logrus"
	etcd "go.etcd.io/etcd/clientv3"
	v3rpc "go.etcd.io/etcd/etcdserver/api/v3rpc/rpctypes"
)

// NewWatcher returns a new watcher to relay changes to the specified key.
// The watcher is automatically started.
func NewWatcher(config WatcherConfig) (*Watcher, error) {
	if err := config.checkAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	w := &Watcher{
		config: config,
		ctx:    ctx,
		cancel: cancel,
		respC:  make(chan etcd.Event),
	}
	go w.loop()
	return w, nil
}

// Watcher can follow and push notifications whenever
// there is a change for the specified key.
type Watcher struct {
	config WatcherConfig
	respC  chan etcd.Event
	ctx    context.Context
	cancel context.CancelFunc
}

// WatcherConfig defines watcher configuration
type WatcherConfig struct {
	// Key specifies the key to watch
	Key string
	// Client specifies the etcd client
	Client       *etcd.Client
	RetryTimeout time.Duration
	logrus.FieldLogger
	clock clockwork.Clock
}

// Stop requests watcher to stop and release resources
func (r *Watcher) Stop() {
	r.cancel()
}

// RespChan returns the channel that the watcher is relaying the updates to.
// The channel must be served to avoid blocking the watcher
func (r *Watcher) RespChan() <-chan etcd.Event {
	return r.respC
}

func (r *Watcher) loop() {
	defer close(r.respC)

	ticker := r.config.clock.NewTicker(r.config.RetryTimeout)
	defer ticker.Stop()
	opts := []etcd.OpOption{
		etcd.WithPrefix(),
	}
	var revision int64

	resp, err := r.config.Client.Get(r.ctx, r.config.Key, opts...)
	if err == nil && len(resp.Kvs) != 0 {
		// Update the receiver
		select {
		case r.respC <- etcd.Event{
			Kv: resp.Kvs[0],
		}:
			revision = resp.Kvs[0].ModRevision + 1
		case <-r.ctx.Done():
			return
		}
	}

	wc := etcd.NewWatcher(r.config.Client)
	defer wc.Close()

	for {
		if err != nil {
			select {
			case <-ticker.Chan():
				r.config.WithError(err).Warn("Retry watcher.")
			case <-r.ctx.Done():
				return
			}
		}

		wopts := []etcd.OpOption{etcd.WithRev(revision)}
		watchC := wc.Watch(r.ctx, r.config.Key, append(wopts, opts...)...)
	W:
		for {
			select {
			case resp, ok := <-watchC:
				if !ok {
					err = trace.LimitExceeded("watcher canceled")
					continue W
				}
				if resp.Canceled && resp.Err() != nil {
					if resp.Err() == v3rpc.ErrCompacted {
						revision = resp.CompactRevision
					}
					err = resp.Err()
					continue W
				}
				if !resp.Created && resp.Header.GetRevision() > revision {
					revision = resp.Header.GetRevision() + 1
				}
				for _, ev := range resp.Events {
					select {
					case r.respC <- *ev:
					case <-r.ctx.Done():
						return
					}
				}

			case <-r.ctx.Done():
				return
			}
		}
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

const retryTimeout = 5 * time.Second
