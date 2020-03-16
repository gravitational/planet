package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/gravitational/planet/lib/constants"
	"github.com/gravitational/planet/lib/reconcile"

	"github.com/gravitational/coordinate/leader"
	"github.com/gravitational/trace"
)

// Name returns the name of this plan.
// Implements reconciler.Plan
func (r *agentPlan) Name() string {
	return "agent state reconciler"
}

// Creates a new plan for agent operation
// Implements reconciler.Plan
func (r *agentPlan) Create(context.Context) (steps []reconcile.Step, err error) {
	if !r.electionPaused {
		steps = append(steps, &newVoter{
			leaderKey: r.leaderKey,
			localAddr: r.localAddr,
			term:      r.electionTerm,
			client:    r.client,
		})
	} else {
		steps = append(steps, &removeVoter{client: r.client})
	}
	steps = append(steps, updateDNS{leaderAddr: r.leaderAddr})
	if r.localAddr == r.leaderAddr {
		return append(steps, startUnits(r.mon)...), nil
	}
	return append(steps, stopUnits(r.mon)...), nil
}

type agentPlan struct {
	electionPaused bool
	electionTerm   time.Duration
	leaderKey      string
	localAddr      string
	leaderAddr     string
	client         *leader.Client
	mon            *unitMonitor
}

// Name returns the name of this step
func (r *newVoter) Name() string {
	return "enable election"
}

// Do starts voting
func (r *newVoter) Do(ctx context.Context) ([]reconcile.Step, error) {
	r.client.AddVoter(ctx, r.leaderKey, r.localAddr, r.term)
	return nil, nil
}

// newVoter implements a step of starting the voter loop
type newVoter struct {
	leaderKey string
	localAddr string
	term      time.Duration
	client    *leader.Client
}

// Name returns the name of this step
func (r *removeVoter) Name() string {
	return "disable election"
}

// Do stops voting
func (r *removeVoter) Do(ctx context.Context) ([]reconcile.Step, error) {
	r.client.RemoveVoter(ctx)
	return nil, nil
}

// removeVoter implements a step of stopping the voter loop
type removeVoter struct {
	client *leader.Client
}

// Name returns the name of this step
func (r updateDNS) Name() string { return "update Coredns configuration" }

// Do updates the Coredns configuration file with the new leader address
func (r updateDNS) Do(ctx context.Context) ([]reconcile.Step, error) {
	if err := writeLocalLeader(constants.CoreDNSHosts, r.leaderAddr); err != nil {
		return nil, trace.Wrap(err)
	}
	return nil, nil
}

type updateDNS struct {
	leaderAddr string
}

func startUnits(mon *unitMonitor) (steps []reconcile.Step) {
	for _, unit := range electedUnits {
		steps = append(steps, newStartUnit(mon, unit))
	}
	return steps
}

func stopUnits(mon *unitMonitor) (steps []reconcile.Step) {
	for _, unit := range electedUnits {
		steps = append(steps, newStopUnit(mon, unit))
	}
	return steps
}

func newStopUnit(mon *unitMonitor, name string) *stopUnit {
	return &stopUnit{
		mon:  mon,
		name: name,
	}
}

// Name returns the name of this service unit
func (r *stopUnit) Name() string {
	return fmt.Sprintf("stop-service(%v)", r.name)
}

// Do stops the service unit and returns a optional step to reset the failed status
// if it fails to stop
func (r *stopUnit) Do(context.Context) ([]reconcile.Step, error) {
	if err := r.mon.stop(r.name); err != nil {
		return nil, trace.Wrap(err, "failed to stop service").AddField("unit", r.name)
	}
	return []reconcile.Step{newResetFailedUnit(r.mon, r.name)}, nil
}

type stopUnit struct {
	mon  *unitMonitor
	name string
}

func newStartUnit(mon *unitMonitor, name string) *startUnit {
	return &startUnit{
		mon:  mon,
		name: name,
	}
}

// Name returns the name of this service unit
func (r *startUnit) Name() string {
	return fmt.Sprintf("start-service(%v)", r.name)
}

// Do starts this service unit
func (r *startUnit) Do(context.Context) ([]reconcile.Step, error) {
	if err := r.mon.start(r.name); err != nil {
		return nil, trace.Wrap(err, "failed to start service").AddField("unit", r.name)
	}
	return nil, nil
}

type startUnit struct {
	mon  *unitMonitor
	name string
}

func newResetFailedUnit(mon *unitMonitor, name string) *startUnit {
	return &startUnit{
		mon:  mon,
		name: name,
	}
}

// Name returns the name of this service unit
func (r *resetFailedUnit) Name() string {
	return fmt.Sprintf("reset-status(%v)", r.name)
}

// Do resets the status of the underlying service unit if it's failed
func (r *resetFailedUnit) Do(context.Context) ([]reconcile.Step, error) {
	unit, err := r.mon.status(r.name)
	if err != nil {
		return nil, trace.Wrap(err, "failed to query service status").AddField("unit", r.name)
	}
	if unit.ActiveState == "stopped" {
		return nil, nil
	}
	if err := r.mon.resetStatus(r.name); err != nil {
		return nil, trace.Wrap(err, "failed to reset service status").AddField("unit", r.name)
	}
	return nil, nil
}

type resetFailedUnit struct {
	mon  *unitMonitor
	name string
}

func writeLocalLeader(target, leaderAddr string) error {
	contents := fmt.Sprintln(leaderAddr, " ",
		constants.APIServerDNSName, " ",
		constants.APIServerDNSNameGravity, " ",
		constants.RegistryDNSName, " ",
		constants.LegacyAPIServerDNSName)
	return trace.ConvertSystemError(ioutil.WriteFile(target, []byte(contents), constants.SharedFileMask))
}

var electedUnits = []string{
	"kube-controller-manager.service",
	"kube-scheduler.service",
	"kube-apiserver.service",
}
