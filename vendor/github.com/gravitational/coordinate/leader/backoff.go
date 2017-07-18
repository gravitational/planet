package leader

import "github.com/cenkalti/backoff"

// NewUnlimitedExponentialBackOff returns a new exponential backoff interval
// w/o time limit
func NewUnlimitedExponentialBackOff() *backoff.ExponentialBackOff {
	b := backoff.NewExponentialBackOff()
	b.MaxElapsedTime = 0
	return b
}
