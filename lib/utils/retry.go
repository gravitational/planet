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
		log.Debugf("attempt %v, result: %v, retry in %v", i+1, err, period)
		select {
		case <-ctx.Done():
			log.Debugf("context is closing, return")
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
		log.Errorf("All attempts failed: %v.", trace.DebugReport(err))
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
