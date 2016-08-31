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
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/gravitational/coordinate/config"
	"github.com/gravitational/trace"

	log "github.com/Sirupsen/logrus"
	ebackoff "github.com/cenk/backoff"
	"github.com/coreos/etcd/client"
	"github.com/jonboulle/clockwork"
	"golang.org/x/net/context"
)

// Config sets leader election configuration options
type Config struct {
	// ETCD defines etcd configuration, client will be instantiated
	// if passed
	ETCD *config.Config
	// Clock is a time provider
	Clock clockwork.Clock
	// Client is ETCD client will be used if passed
	Client client.Client
}

// Client implements ETCD-backed leader election client
// that helps to elect new leaders for a given key and
// monitors the changes to the leaders
type Client struct {
	client client.Client
	clock  clockwork.Clock
	closeC chan bool
	closed uint32
}

// NewClient returns a new instance of leader election client
func NewClient(cfg Config) (*Client, error) {
	if cfg.Clock == nil {
		cfg.Clock = clockwork.NewRealClock()
	}
	var err error
	client := cfg.Client
	if client == nil {
		if cfg.ETCD == nil {
			return nil, trace.BadParameter("expected either ETCD config or Client")
		}
		if err = cfg.ETCD.CheckAndSetDefaults(); err != nil {
			return nil, trace.Wrap(err)
		}
		client, err = cfg.ETCD.NewClient()
		if err != nil {
			return nil, trace.Wrap(err)
		}
	}
	return &Client{
		client: client,
		clock:  cfg.Clock,
		closeC: make(chan bool),
	}, nil
}

// CallbackFn specifies callback that is called by AddWatchCallback
// whenever leader changes
type CallbackFn func(key, prevValue, newValue string)

// AddWatchCallback adds the given callback to be invoked when changes are
// made to the specified key's value. The callback is called with new and
// previous values for the key. In the first call, both values are the same
// and reflect the value of the key at that moment
func (l *Client) AddWatchCallback(key string, retry time.Duration, fn CallbackFn) {
	go func() {
		valuesC := make(chan string)
		l.AddWatch(key, retry, valuesC)
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

func (l *Client) getWatchAtLatestIndex(ctx context.Context, api client.KeysAPI, key string, retry time.Duration) (client.Watcher, *client.Response, error) {
	re, err := l.getFirstValue(key, retry)
	if err != nil {
		return nil, nil, trace.Errorf("%v unexpected error: %v, returning", ctx.Value("prefix"), err)
	} else if re == nil {
		log.Infof("%v client is closing, return", ctx.Value("prefix"))
		return nil, nil, nil
	}
	log.Infof("%v got current value '%v' for key '%v'", ctx.Value("prefix"), re.Node.Value, key)
	watcher := api.Watcher(key, &client.WatcherOptions{
		AfterIndex: re.Node.ModifiedIndex,
	})
	return watcher, re, nil
}

// AddWatch starts watching the key for changes and sending them
// to the valuesC, the watch is stopped
func (l *Client) AddWatch(key string, retry time.Duration, valuesC chan string) {
	prefix := fmt.Sprintf("AddWatch(key=%v)", key)
	api := client.NewKeysAPI(l.client)

	go func() {
		var watcher client.Watcher
		var re *client.Response
		var err error

		ctx, closer := context.WithCancel(context.WithValue(context.Background(), "prefix", prefix))
		go func() {
			<-l.closeC
			closer()
		}()

		backoff := &FreebieExponentialBackOff{
			InitialInterval:     500 * time.Millisecond,
			RandomizationFactor: 0.5,
			Multiplier:          1.5,
			MaxInterval:         10 * time.Second,
			MaxElapsedTime:      30 * time.Second,
		}
		ticker := ebackoff.NewTicker(backoff)

		watcher, re, err = l.getWatchAtLatestIndex(ctx, api, key, retry)
		if err != nil {
			return
		}

		var sentAnything bool
		for {
			re, err = watcher.Next(ctx)
			if err == nil {
				if re.Node.Value == "" {
					log.Infof("watcher.Next for %v skipping empty value", key)
					continue
				}
				log.Infof("watcher.Next for %v got %v", key, re.Node.Value)
				backoff.Reset()
			}
			if err != nil {
				select {
				case b := <-ticker.C:
					log.Infof("%v backoff %v", prefix, b)
				}

				if err == context.Canceled {
					log.Infof("client is closing, return")
					return
				} else if cerr, ok := err.(*client.ClusterError); ok {
					if len(cerr.Errors) != 0 && cerr.Errors[0] == context.Canceled {
						log.Infof("client is closing, return")
						return
					}
					log.Infof("unexpected cluster error: %v (%v)", err, cerr.Detail())
					continue
				} else if cerr, ok := err.(client.Error); ok && cerr.Code == client.ErrorCodeEventIndexCleared {
					log.Infof("watch index error, resetting watch index: %v", cerr)
					watcher, re, err = l.getWatchAtLatestIndex(ctx, api, key, retry)
					if err != nil {
						continue
					}
				} else {
					log.Infof("unexpected watch error: %v", err)
					// try recreating the watch if we get repeated unknown errors
					if backoff.CurrentTries() > 10 {
						watcher, re, err = l.getWatchAtLatestIndex(ctx, api, key, retry)
						if err != nil {
							continue
						}
						backoff.Reset()
					}

					continue
				}
			}
			// if nothing has changed and we previously sent this subscriber this value,
			// do not bother subscriber with extra notifications
			if re.PrevNode != nil && re.PrevNode.Value == re.Node.Value && sentAnything {
				continue
			}
			select {
			case valuesC <- re.Node.Value:
				sentAnything = true
			case <-l.closeC:
				return
			}
		}
	}()
}

// AddVoter starts a goroutine that attempts to set the specified key to
// to the given value with the time-to-live value specified with term.
// The time-to-live value cannot be less than a second.
// After successfully setting the key, it attempts to renew the lease for the specified
// term indefinitely
func (l *Client) AddVoter(key, value string, term time.Duration) (io.Closer, error) {
	if value == "" {
		return nil, trace.Errorf("voter value for key cannot be empty")
	}
	if term < time.Second {
		return nil, trace.Errorf("term cannot be < 1s")
	}
	voter := &voter{doneC: make(chan struct{})}
	go func() {
		err := l.elect(key, value, term)
		if err != nil {
			log.Infof("voter error: %v", err)
		}
		ticker := time.NewTicker(term / 5)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				err := l.elect(key, value, term)
				if err != nil {
					log.Infof("voter error: %v", err)
				}
			case <-l.closeC:
				log.Infof("client is closing, return")
				return
			case <-voter.doneC:
				log.Infof("removing voter for %v", value)
				return
			}
		}
	}()
	return voter, nil
}

// voter defines a handle to single voter process
type voter struct {
	doneC chan struct{}
}

// Close terminates the specified voter process
func (r *voter) Close() error {
	close(r.doneC)
	return nil
}

// getFirstValue returns the current value for key if it exists, or waits
// for the value to appear and loops until client.Close is called
func (l *Client) getFirstValue(key string, retryPeriod time.Duration) (*client.Response, error) {
	api := client.NewKeysAPI(l.client)
	tick := time.NewTicker(retryPeriod)
	defer tick.Stop()
	for {
		re, err := api.Get(context.TODO(), key, nil)
		if err == nil {
			return re, nil
		} else if !IsNotFound(err) {
			log.Infof("unexpected watcher error: %v", err)
		}
		select {
		case <-tick.C:
		case <-l.closeC:
			log.Infof("watcher got client close signal")
			return nil, nil
		}
	}
}

// elect is taken from: https://github.com/kubernetes/contrib/blob/master/pod-master/podmaster.go
// this is a slightly modified version though, that does not return the result
// instead we rely on watchers
func (l *Client) elect(key, value string, term time.Duration) error {
	candidate := fmt.Sprintf("candidate(key=%v, value=%v, term=%v)", key, value, term)
	log.Infof("%v start", candidate)
	api := client.NewKeysAPI(l.client)
	resp, err := api.Get(context.TODO(), key, nil)
	if err != nil {
		if !IsNotFound(err) {
			return trace.Wrap(err)
		}
		log.Infof("%v key not found, try to elect myself", candidate)
		// try to grab the lock for the given term
		_, err := api.Set(context.TODO(), key, value, &client.SetOptions{
			TTL:       term,
			PrevExist: client.PrevNoExist,
		})
		if err != nil {
			return trace.Wrap(err)
		}
		log.Infof("%v successfully elected", candidate)
		return nil
	}
	if resp.Node.Value != value {
		log.Infof("%v: leader is %v, try again", candidate, resp.Node.Value)
		return nil
	}
	if resp.Node.Expiration.Sub(l.clock.Now().UTC()) > time.Duration(term/2) {
		return nil
	}

	// extend the lease before the current expries
	_, err = api.Set(context.TODO(), key, value, &client.SetOptions{
		TTL:       term,
		PrevValue: value,
		PrevIndex: resp.Node.ModifiedIndex,
	})
	if err != nil {
		return trace.Wrap(err)
	}
	log.Infof("%v extended lease", candidate)
	return nil
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

func IsNotFound(err error) bool {
	e, ok := err.(client.Error)
	if !ok {
		return false
	}
	return e.Code == client.ErrorCodeKeyNotFound
}

func IsAlreadyExist(err error) bool {
	e, ok := err.(client.Error)
	if !ok {
		return false
	}
	return e.Code == client.ErrorCodeNodeExist
}
