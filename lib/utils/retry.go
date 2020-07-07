/*
Copyright 2018 Gravitational, Inc.

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

package utils

import (
	"context"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

// Retry retries 'times' attempts with retry period 'period' calling function fn
// until it returns nil, or until the context gets cancelled or the retries
// get exceeded the times number of attempts
func Retry(ctx context.Context, times int, period time.Duration, fn func() error) error {
	var err error
	for i := 0; i < times; i += 1 {
		err = fn()
		if err == nil {
			return nil
		}
		log.Debugf("Attempt %v, result: %v, retry in %v", i+1, err, period)
		select {
		case <-ctx.Done():
			log.Debug("Context is closing, return.")
			return err
		case <-time.After(period):
		}
	}
	return trace.Wrap(err)
}

// RetryWithInterval retries the specified operation fn using the specified backoff interval.
// fn should return backoff.PermanentError if the error should not be retried and returned directly.
// Returns nil on success or the last received error upon exhausting the interval.
func RetryWithInterval(ctx context.Context, interval backoff.BackOff, fn func() error) error {
	b := backoff.WithContext(interval, ctx)
	err := backoff.RetryNotify(func() (err error) {
		err = fn()
		return err
	}, b, func(err error, d time.Duration) {
		log.Debugf("Retrying: %v (time %v).", trace.UserMessage(err), d)
	})
	if err != nil {
		log.WithError(err).Warn("All attempts failed.")
		return trace.Wrap(err)
	}
	return nil
}

// NewUnlimitedExponentialBackOff returns a backoff interval without time restriction
func NewUnlimitedExponentialBackOff() backoff.BackOff {
	b := backoff.NewExponentialBackOff()
	b.MaxElapsedTime = 0
	return b
}

// Error returns the contents of this value.
// Implements error
func (r Continue) Error() string {
	return string(r)
}

// Continue is an artificial error value that indicates that a retry
// loop should be re-executed with the specified message
type Continue string
