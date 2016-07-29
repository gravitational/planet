package leader

import (
	"math"
	"math/rand"
	"time"

	"github.com/cenk/backoff"
)

// FreebieExponentialBackOff
type FreebieExponentialBackOff struct {
	InitialInterval     time.Duration
	RandomizationFactor float64
	Multiplier          float64
	MaxInterval         time.Duration
	MaxElapsedTime      time.Duration

	tries   int
	elapsed time.Duration
}

// NextBackOff returns the duration of the next backoff interval
func (f *FreebieExponentialBackOff) NextBackOff() time.Duration {
	f.tries++
	if f.tries == 1 {
		return time.Duration(0)
	} else {
		delay := float64(f.InitialInterval) * math.Pow(f.Multiplier, float64(f.tries-2))
		jitter := (rand.Float64() - 0.5) * f.RandomizationFactor * float64(f.InitialInterval)
		delay += jitter
		if delay >= float64(f.MaxInterval) {
			delay = float64(f.MaxInterval)
		}
		f.elapsed += time.Duration(delay)
		if f.elapsed > f.MaxElapsedTime {
			return backoff.Stop
		}
		return time.Duration(delay)
	}
}

// Reset resets the number of tries on this backoff counter to zero
func (f *FreebieExponentialBackOff) Reset() {
	f.tries = 0
	f.elapsed = time.Duration(0)
}

// CurrentTries returns the number of attempts on this backoff counter
func (f *FreebieExponentialBackOff) CurrentTries() int {
	return f.tries
}
