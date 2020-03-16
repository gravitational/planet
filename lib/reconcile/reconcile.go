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
	"sync"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/gravitational/trace"
	"github.com/sirupsen/logrus"
)

// Plan describes a plan as a list of steps to achieving a specific goal.
// Plan is complete when the next call to Create does not produce any steps
type Plan interface {
	// Name returns the name of this plan.
	// The name is used in logging
	Name() string
	// Create creates a list of operation steps to execute as part of the plan.
	// Individual steps are capable of generating sub-steps as part of their work.
	// Once this returns an empty list, the plan is considered complete
	Create(context.Context) ([]Step, error)
}

// Step describes a single reconciler step
type Step interface {
	// Name returns the step name
	Name() string
	// Do executes the step action.
	// Optionally returns additional Steps to execute or error in case of failure
	Do(context.Context) ([]Step, error)
}

// New creates a new plan reconciler
func New() *Reconciler {
	return &Reconciler{
		log:         logrus.WithField(trace.Component, "reconciler"),
		stop:        make(chan chan struct{}),
		planUpdates: make(chan Plan, 1),
	}
}

// Reset strarts the reconciler loop using the specified plan.
// If the reconciler is already executing a plan, it will be canceled to start a new one
func (r *Reconciler) Reset(ctx context.Context, plan Plan) {
	r.once.Do(func() {
		go r.loop()
	})
	select {
	case <-ctx.Done():
	case r.planUpdates <- plan:
	}
}

// Stop stops the reconciler loop/
// Stop cannot be invoked multiple times
func (r *Reconciler) Stop() {
	c := make(chan struct{})
	r.stop <- c
	<-c
}

// Reconciler is a plan reconciler
type Reconciler struct {
	log         logrus.FieldLogger
	stop        chan chan struct{}
	once        sync.Once
	planUpdates chan Plan
}

func (r *Reconciler) loop() {
	const timeout = 10 * time.Second
	var plan Plan
	var ticker *backoff.Ticker
	var tickerC <-chan time.Time
	ctx, cancel := context.WithCancel(context.Background())
	for {
		select {
		case <-tickerC:
			// Another iteration
			logger := r.log.WithField("plan", plan.Name())
			steps, err := plan.Create(ctx)
			if err != nil {
				logger.WithError(err).Warn("Failed to create plan.")
				continue
			}
			if err := execute(ctx, steps, logger); err != nil {
				logger.WithError(err).Warn("Failed to execute plan.")
				continue
			}
			tickerC = nil
			ticker.Stop()
			cancel()

		case c := <-r.stop:
			cancel()
			r.stop = nil
			close(c)
			return

		case plan = <-r.planUpdates:
			if ticker != nil {
				ticker.Stop()
				tickerC = nil
			}
			cancel()
			ticker = backoff.NewTicker(backoff.NewConstantBackOff(timeout))
			tickerC = ticker.C
		}
	}
}

func execute(ctx context.Context, steps []Step, logger logrus.FieldLogger) error {
	for _, step := range steps {
		logger := logger.WithField("step", step.Name())
		logger.Info("Execute step.")
		substeps, err := step.Do(ctx)
		if err != nil {
			return trace.Wrap(err)
		}
		if err := execute(ctx, substeps, logger); err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}
