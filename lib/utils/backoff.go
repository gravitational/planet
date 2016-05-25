package utils

import (
	"math"
	"time"
)

// Backoff is an exponential counter that gives increasing time.Duration values up to Max.
type Backoff struct {
	Initial time.Duration
	Max     time.Duration
	Tries   int
}

// Reset sets the number of tries back to zero (i.e. successful case)
func (b *Backoff) Reset() {
	b.Tries = 0
}

// Delay returns the next time.Duration to wait between successive attempts.
func (b *Backoff) Delay() time.Duration {
	delay := float64(b.Initial) * math.Pow(2, float64(b.Tries))
	b.Tries += 1
	if delay > float64(b.Max) {
		return b.Max
	}
	return time.Duration(delay)
}
