package utils

import (
	"context"
	"time"

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
