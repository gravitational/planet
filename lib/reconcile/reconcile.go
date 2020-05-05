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

	"github.com/gravitational/trace"
	"github.com/jonboulle/clockwork"
	"github.com/sirupsen/logrus"
)

// Plan describes a plan as a list of steps to achieving a specific goal.
// Plan is complete when all steps execute without errors.
type Plan interface {
	// Name returns the name of this plan.
	// The name is used in logging
	Name() string
	// Create creates a list of operation steps to execute as part of the plan.
	// Individual steps are capable of generating sub-steps as part of their work.
	Create(context.Context) ([]Step, error)
}

// Step describes a single reconciler step
type Step interface {
	// Name returns the step name
	Name() string
	// Do executes the step.
	// Optionally returns additional Steps to execute or error in case of failure
	Do(context.Context) ([]Step, error)
}

// New creates a new plan reconciler
func New(config Config) *Reconciler {
	config.setDefaults()
	return &Reconciler{
		config:      config,
		log:         logrus.WithField(trace.Component, "reconciler"),
		stop:        make(chan chan struct{}),
		planUpdates: make(chan Plan, 1),
	}
}

// Reset starts the reconciler loop using the specified plan.
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
	config      Config
	log         logrus.FieldLogger
	stop        chan chan struct{}
	once        sync.Once
	planUpdates chan Plan
}

// Config defines reconciler configuration
type Config struct {
	// Timeout defines the time steps for executing a single plan between attempts
	Timeout time.Duration
	// ResyncTimeout defines the maximum time to wait between unconditional attempts
	// to execute the last plan
	ResyncTimeout time.Duration
	// clock specifies the time implementation.
	// Overridden in tests
	clock clockwork.Clock
}

func (r *Config) setDefaults() {
	if r.Timeout == 0 {
		r.Timeout = timeout
	}
	if r.ResyncTimeout == 0 {
		r.ResyncTimeout = resyncTimeout
	}
	if r.clock == nil {
		r.clock = clockwork.NewRealClock()
	}
}

func (r *Reconciler) loop() {
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	for {
		select {
		case c := <-r.stop:
			r.log.Debug("Stop.")
			r.stop = nil
			cancel()
			wg.Wait()
			close(c)
			return

		case plan := <-r.planUpdates:
			cancel()
			wg.Wait()
			ctx, cancel = context.WithCancel(context.Background())
			wg.Add(1)
			go func() {
				r.executorLoop(ctx, plan)
				wg.Done()
			}()
		}
	}
}

func (r *Reconciler) executorLoop(ctx context.Context, plan Plan) {
	timeout := r.config.Timeout
	if r.executePlan(ctx, plan) == nil {
		timeout = r.config.ResyncTimeout
	}
	ticker := r.config.clock.NewTicker(timeout)
	defer ticker.Stop()
	tickerC := ticker.Chan()
	for {
		select {
		case <-tickerC:
			if r.executePlan(ctx, plan) != nil {
				if timeout != r.config.Timeout {
					ticker.Stop()
					timeout = r.config.Timeout
					ticker = r.config.clock.NewTicker(timeout)
					tickerC = ticker.Chan()
				}
				continue
			}
			ticker.Stop()
			ticker = r.config.clock.NewTicker(r.config.ResyncTimeout)
			tickerC = ticker.Chan()

		case <-ctx.Done():
			return
		}
	}
}

func (r *Reconciler) executePlan(ctx context.Context, plan Plan) error {
	logger := r.log.WithField("plan", plan.Name())
	logger.Info("Reconcile plan.")
	steps, err := plan.Create(ctx)
	if err != nil {
		logger.WithError(err).Warn("Failed to create plan.")
		return trace.Wrap(err)
	}
	if err := execute(ctx, steps, logger); err != nil {
		logger.WithError(err).Warn("Failed to execute plan.")
		return trace.Wrap(err)
	}
	logger.Info("Reconciled plan.")
	return nil
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

const (
	timeout       = 10 * time.Second
	resyncTimeout = 5 * time.Minute
)
