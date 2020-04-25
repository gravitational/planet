/*
Copyright 2020 Gravitational, Inc.

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

package reconcile

import (
	"context"
	"os"
	"testing"

	"github.com/gravitational/trace"
	"github.com/jonboulle/clockwork"
	log "github.com/sirupsen/logrus"
	check "gopkg.in/check.v1"
)

func TestReconciler(t *testing.T) { check.TestingT(t) }

type S struct{}

var _ = check.Suite(&S{})

func (*S) SetUpTest(c *check.C) {
	log.SetLevel(log.DebugLevel)
	log.SetOutput(os.Stderr)
}

func (*S) TestCanStartStopReconciler(c *check.C) {
	r := New(Config{})
	defer r.Stop()
	done := make(chan struct{})
	r.Reset(context.Background(), &plan{
		steps: []Step{
			&step{done: done, name: "step"},
		},
	})
	<-done
}

func (*S) TestExecutesTestAfterResyncTimeout(c *check.C) {
	clock := clockwork.NewFakeClock()
	r := New(Config{
		clock: clock,
	})
	defer r.Stop()
	done := make(chan struct{}, 2)
	r.Reset(context.Background(), &plan{
		steps: []Step{
			&step{done: done, name: "step"},
		},
	})
	<-done
	clock.BlockUntil(1)
	// resync timeout
	clock.Advance(resyncTimeout)
	clock.BlockUntil(1)
	<-done
}

func (*S) TestRepeatsExecutionOnFailure(c *check.C) {
	clock := clockwork.NewFakeClock()
	r := New(Config{
		clock: clock,
	})
	defer r.Stop()
	done := make(chan struct{}, 2)
	r.Reset(context.Background(), &plan{
		steps: []Step{
			&step{
				done:     done,
				name:     "step",
				failures: 1,
				err:      trace.BadParameter("step failed"),
			},
		},
	})
	<-done
	clock.BlockUntil(1)
	// timeout after error
	clock.Advance(timeout)
	clock.BlockUntil(1)
	<-done
}

func (r *plan) Name() string {
	return "test plan"
}

func (r *plan) Create(context.Context) ([]Step, error) {
	return r.steps, r.err
}

type plan struct {
	steps []Step
	err   error
}

func (r *step) Name() string { return r.name }
func (r *step) Do(context.Context) ([]Step, error) {
	defer func() {
		r.done <- struct{}{}
	}()
	if r.failures > 0 {
		r.failures--
		return nil, r.err
	}
	return nil, nil
}

type step struct {
	name     string
	done     chan struct{}
	failures int
	err      error
}
