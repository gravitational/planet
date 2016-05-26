package utils

import (
	"time"

	cbackoff "github.com/cenk/backoff"
	. "gopkg.in/check.v1"
)

type BackOffSuite struct {
}

var _ = Suite(&BackOffSuite{})

type TestClock struct {
	i     time.Duration
	start time.Time
}

func (c *TestClock) Now() time.Time {
	t := c.start.Add(c.i)
	c.i += time.Second
	return t
}

func (s *BackOffSuite) TestBackOff(c *C) {
	backoff := &FreebieExponentialBackOff{
		InitialInterval:     500 * time.Millisecond,
		RandomizationFactor: 0,
		Multiplier:          2,
		MaxInterval:         10 * time.Second,
		MaxElapsedTime:      40 * time.Second,
	}

	c.Assert(backoff.NextBackOff(), Equals, time.Duration(0))
	c.Assert(backoff.NextBackOff(), Equals, 500*time.Millisecond)
	c.Assert(backoff.CurrentTries(), Equals, 2)

	backoff.Reset()
	c.Assert(backoff.CurrentTries(), Equals, 0)
	c.Assert(backoff.NextBackOff(), Equals, time.Duration(0))
	c.Assert(backoff.NextBackOff(), Equals, 500*time.Millisecond)
	c.Assert(backoff.NextBackOff(), Equals, 1000*time.Millisecond)
	c.Assert(backoff.NextBackOff(), Equals, 2000*time.Millisecond)
	c.Assert(backoff.NextBackOff(), Equals, 4000*time.Millisecond)
	c.Assert(backoff.NextBackOff(), Equals, 8000*time.Millisecond)
	c.Assert(backoff.NextBackOff(), Equals, 10000*time.Millisecond)
	c.Assert(backoff.NextBackOff(), Equals, 10000*time.Millisecond)
	c.Assert(backoff.NextBackOff(), Equals, cbackoff.Stop)
}
