/*
Copyright 2016 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package leader

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/gravitational/trace"
	"github.com/sirupsen/logrus"
	"go.etcd.io/etcd/client"
)

var log = logrus.WithField(trace.Component, "leader")

// Client implements ETCD-backed leader election client
// that helps to elect new leaders for a given key and
// monitors the changes to the leaders
type Client struct {
	Config
	closeC chan struct{}
	pauseC chan bool
	closed uint32
	// voterC controls the voting participation
	voterC chan bool
	once   sync.Once
}

// NewClient returns a new instance of leader election client
func NewClient(cfg Config) (*Client, error) {
	if err := cfg.checkAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	return &Client{
		Config: cfg,
		closeC: make(chan struct{}),
		pauseC: make(chan bool),
		voterC: make(chan bool),
	}, nil
}

func (r *Config) checkAndSetDefaults() error {
	if r.Client == nil {
		return trace.BadParameter("Client is required")
	}
	return nil
}

// Config sets leader election configuration options
type Config struct {
	// Client is the ETCD client to use
	Client client.Client
}

// CallbackFn specifies callback that is called by AddWatchCallback
// whenever leader changes
type CallbackFn func(key, prevValue, newValue string)

// AddWatchCallback adds the given callback to be invoked when changes are
// made to the specified key's value. The callback is called with new and
// previous values for the key. In the first call, both values are the same
// and reflect the value of the key at that moment
func (l *Client) AddWatchCallback(key string, fn CallbackFn) {
	go func() {
		valuesC := make(chan string)
		l.AddWatch(key, valuesC)
		var prev string
		for {
			select {
			case <-l.closeC:
				return
			case val := <-valuesC:
				fn(key, prev, val)
				prev = val
			}
		}
	}()
}

// AddWatch starts watching the key for changes and sending them
// to the valuesC until the client is stopped.
func (l *Client) AddWatch(key string, valuesC chan string) {
	logger := log.WithField("key", key)
	logger.WithField("peers", l.Client.Endpoints()).Info("Setting up watch.")

	go l.watchLoop(key, valuesC, logger)
}

// AddVoter starts a goroutine that attempts to set the specified key to
// to the given value with the time-to-live value specified with term.
// The time-to-live value cannot be less than a second.
// After successfully setting the key, it attempts to renew the lease for the specified
// term indefinitely.
// The method is idempotent and does nothing if invoked multiple times
func (l *Client) AddVoter(ctx context.Context, key, value string, term time.Duration) {
	l.once.Do(func() {
		go l.voterLoop(key, value, term, true)
	})
	select {
	case l.voterC <- true:
	case <-ctx.Done():
	}
}

// RemoveVoter stops the voting loop.
func (l *Client) RemoveVoter(ctx context.Context, key, value string, term time.Duration) {
	l.once.Do(func() {
		go l.voterLoop(key, value, term, false)
	})
	select {
	case l.voterC <- false:
	case <-ctx.Done():
	}
}

// StepDown makes this participant to pause his attempts to re-elect itself thus giving up its leadership
func (l *Client) StepDown(ctx context.Context) {
	select {
	case l.pauseC <- true:
	case <-ctx.Done():
	}
}

// Close stops current operations and releases resources
func (l *Client) Close() error {
	// already closed
	if !atomic.CompareAndSwapUint32(&l.closed, 0, 1) {
		return nil
	}
	close(l.closeC)
	return nil
}

func (l *Client) watchLoop(key string, valuesC chan string, logger logrus.FieldLogger) {
	api := client.NewKeysAPI(l.Client)
	var watcher client.Watcher
	var resp *client.Response
	var err error

	b := NewUnlimitedExponentialBackOff()
	ticker := backoff.NewTicker(b)
	defer ticker.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-l.closeC
		cancel()
	}()

	// steps collects number of failed attempts due to unknown error so far
	var steps int
	tick := func(err error) (ok bool) {
		if err == nil {
			return true
		}
		if IsContextCanceled(err) {
			// The context has been canceled while watcher was waiting
			// for next event.
			logger.Info("Context has been canceled.")
			return false
		}
		select {
		case <-l.closeC:
			return false
		case <-ticker.C:
			if clusterErr, ok := err.(*client.ClusterError); ok {
				// Got an etcd error, the watcher will retry.
				logger.WithError(clusterErr).Errorf("Etcd error: %v.", clusterErr.Detail())
			} else if IsWatchExpired(err) {
				// The watcher has expired, reset it so it's recreated on the
				// next loop cycle.
				logger.Info("Watch has expired, resetting watch index.")
				watcher = nil
			} else {
				steps += 1
				// Unknown error, try recreating the watch after a few repeated
				// unknown errors.
				logger.WithError(err).Error("Unexpected watch error.")
				if steps > 10 {
					watcher = nil
					b.Reset()
					steps = 0
				}
			}
			return true
		}
	}
	for tick(err) {
		if watcher == nil {
			watcher, err = l.getWatchAtLatestIndex(ctx, api, key, valuesC)
			if err != nil {
				continue
			}
			// Successful return means the current value has been sent to receiver
		}
		resp, err = watcher.Next(ctx)
		if err != nil {
			continue
		}
		if resp.Node.Value == "" {
			continue
		}
		if resp.PrevNode != nil && resp.PrevNode.Value == resp.Node.Value {
			continue
		}
		select {
		case valuesC <- resp.Node.Value:
		case <-l.closeC:
			logger.Info("Watcher is closing.")
			return
		}
	}
}

// voterLoop is a process that attempts to set the specified key to
// to the given value with the time-to-live value specified with term.
// The time-to-live value cannot be less than a second.
// After successfully setting the key, it attempts to renew the lease for the specified
// term indefinitely.
func (l *Client) voterLoop(key, value string, term time.Duration, enabled bool) {
	logger := log.WithFields(logrus.Fields{
		"key":   key,
		"value": value,
		"term":  term,
	})
	var ticker *time.Ticker
	var tickerC <-chan time.Time
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-l.closeC
		cancel()
	}()
	if enabled {
		err := l.elect(ctx, key, value, term, logger)
		if err != nil {
			logger.WithError(err).Warn("Failed to run election term.")
		}
		ticker = time.NewTicker(term / 5)
		tickerC = ticker.C
	}
	for {
		select {
		case <-l.pauseC:
			logger.Info("Step down.")
			select {
			case <-time.After(term * 3):
			case <-l.closeC:
				return
			}
		default:
		}

		select {
		case <-tickerC:
			err := l.elect(ctx, key, value, term, logger)
			if err != nil {
				logger.WithError(err).Warn("Failed to run election term.")
			}

		case enabled := <-l.voterC:
			if !enabled {
				if ticker != nil {
					ticker.Stop()
				}
				ticker = nil
				tickerC = nil
				continue
			}
			if tickerC == nil {
				ticker = time.NewTicker(term / 5)
				tickerC = ticker.C
			}

		case <-l.closeC:
			if ticker != nil {
				ticker.Stop()
			}
			return
		}
	}
}

func (l *Client) getWatchAtLatestIndex(ctx context.Context, api client.KeysAPI, key string, valuesC chan string) (client.Watcher, error) {
	logger := log.WithField("key", key)
	logger.Info("Recreating watch at the latest index.")
	resp, err := api.Get(ctx, key, nil)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	// After reestablishing the watch, always send the value we got to the client.
	if resp.Node != nil {
		logger.WithFields(logrus.Fields{
			"value": resp.Node.Value,
			"index": resp.Index,
		}).Info("Got current value.")
		select {
		case valuesC <- resp.Node.Value:
		case <-l.closeC:
			return nil, trace.LimitExceeded("client closed")
		}
	}
	// The watcher that will be receiving events after the value we got above.
	watcher := api.Watcher(key, &client.WatcherOptions{
		// Response.Index corresponds to X-Etcd-Index response header field
		// and is the recommended starting point after a history miss of over
		// 1000 events
		AfterIndex: resp.Index,
	})
	return watcher, nil
}

// elect is taken from: https://github.com/kubernetes/contrib/blob/master/pod-master/podmaster.go
// this is a slightly modified version though, that does not return the result
// instead we rely on watchers
func (l *Client) elect(ctx context.Context, key, value string, term time.Duration, logger logrus.FieldLogger) error {
	api := client.NewKeysAPI(l.Client)
	resp, err := api.Get(ctx, key, nil)
	if err != nil {
		if !IsNotFound(err) {
			return trace.Wrap(err)
		}
		// try to grab the lock for the given term
		_, err := api.Set(ctx, key, value, &client.SetOptions{
			TTL:       term,
			PrevExist: client.PrevNoExist,
		})
		if err != nil {
			return trace.Wrap(err)
		}
		logger.Info("Acquired lease.")
		return nil
	}
	if resp.Node.Value != value {
		return nil
	}
	if resp.Node.Expiration.Sub(time.Now().UTC()) > time.Duration(term/2) {
		return nil
	}

	// extend the lease before the current expries
	_, err = api.Set(ctx, key, value, &client.SetOptions{
		TTL:       term,
		PrevValue: value,
		PrevIndex: resp.Node.ModifiedIndex,
	})
	if err != nil {
		return trace.Wrap(err)
	}
	logger.Info("Extended lease.")
	return nil
}
