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

package main

import (
	"bufio"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gravitational/planet/lib/constants"
	"github.com/gravitational/planet/lib/reconcile"

	"github.com/gravitational/coordinate/leader"
	"github.com/gravitational/trace"
	"github.com/sirupsen/logrus"
)

func newAgentPlan(config LeaderConfig, client *leader.Client, mon *unitMonitor) agentPlan {
	return agentPlan{
		leaderKey:          config.LeaderKey,
		localAddr:          config.PublicIP,
		electionTerm:       config.Term,
		electionPaused:     !config.ElectionEnabled,
		mon:                mon,
		client:             client,
		verifyLeaderAddr:   verifyLeaderAddr,
		verifyServiceState: verifyServiceState,
	}
}

// Name returns the name of this plan.
// Implements reconciler.Plan
func (r *agentPlan) Name() string {
	return "agent state reconciler"
}

// Create creates a new plan for agent operation.
// Implements reconciler.Plan
func (r *agentPlan) Create(ctx context.Context) (steps []reconcile.Step, err error) {
	logger := logrus.WithField(trace.Component, r.Name())
	if !r.electionPaused {
		logger.Debug("Start election participation.")
		r.addVoter(ctx)
	} else {
		logger.Debug("Stop election participation.")
		r.removeVoter(ctx)
	}
	if r.leaderAddr == "" {
		return steps, nil
	}
	if err := r.verifyLeaderAddr(r.leaderAddr); err != nil {
		logger.WithError(err).Warn("Failed to check local leader DNS configuration, will update configuration.")
		steps = append(steps, updateDNS{leaderAddr: r.leaderAddr})
	}
	stateTarget := regularNodeUnitStateTarget
	if r.localAddr == r.leaderAddr {
		stateTarget = leaderNodeUnitStateTarget
	}
	unitsToStop, err1 := r.verifyServiceState(r.mon, stateTarget.stop.state, stateTarget.stop.units, logger)
	unitsToStart, err2 := r.verifyServiceState(r.mon, stateTarget.start.state, stateTarget.start.units, logger)
	if err1 == nil && err2 == nil {
		return steps, nil
	}
	logger.WithError(err).Warn("Failed to verify service state, will reconcile.")
	if len(unitsToStop) != 0 {
		steps = append(steps, stopUnits(r.mon, unitsToStop...)...)
	}
	if len(unitsToStart) != 0 {
		steps = append(steps, startUnits(r.mon, unitsToStart...)...)
	}
	return steps, nil
}

func (r *agentPlan) String() string {
	return fmt.Sprintf("agent-plan(elections=%v, addr=%v, leader-addr=%v)",
		!r.electionPaused,
		r.localAddr,
		r.leaderAddr,
	)
}

type agentPlan struct {
	electionPaused bool
	electionTerm   time.Duration
	leaderKey      string
	localAddr      string
	leaderAddr     string
	client         coordinateClient
	mon            *unitMonitor

	// for testing purposes
	verifyLeaderAddr   func(addr string) error
	verifyServiceState func(mon *unitMonitor, state serviceActiveState, units []string, logger logrus.FieldLogger) ([]string, error)
}

func (r *agentPlan) addVoter(ctx context.Context) {
	r.client.AddVoter(ctx, r.leaderKey, r.localAddr, r.electionTerm)
}

func (r *agentPlan) removeVoter(ctx context.Context) {
	r.client.RemoveVoter(ctx, r.leaderKey, r.localAddr, r.electionTerm)
}

type agentState struct {
	mu   sync.Mutex
	plan agentPlan
}

// withLeaderAddr updates the leader address with the specified value
// and returns a pointer to a copy of the new plan
func (r *agentState) withLeaderAddr(addr string) *agentPlan {
	r.mu.Lock()
	r.plan.leaderAddr = addr
	plan := r.plan
	r.mu.Unlock()
	return &plan
}

// withElectionEnabled updates the election state with the specified value
// and returns a pointer to a copy of the new plan
func (r *agentState) withElectionEnabled(enabled bool) *agentPlan {
	r.mu.Lock()
	r.plan.electionPaused = !enabled
	plan := r.plan
	r.mu.Unlock()
	return &plan
}

// Name returns the name of this step
func (r updateDNS) Name() string {
	return fmt.Sprintf("coredns-config(leader=%v)", r.leaderAddr)
}

// Do updates the Coredns configuration file with the new leader address
func (r updateDNS) Do(ctx context.Context) ([]reconcile.Step, error) {
	if err := writeLocalLeader(r.leaderAddr); err != nil {
		return nil, trace.Wrap(err)
	}
	return nil, nil
}

type updateDNS struct {
	leaderAddr string
}

func startUnits(mon *unitMonitor, units ...string) (steps []reconcile.Step) {
	for _, unit := range units {
		steps = append(steps, newStartUnit(mon, unit))
	}
	return steps
}

func stopUnits(mon *unitMonitor, units ...string) (steps []reconcile.Step) {
	for _, unit := range units {
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
		if trace.IsNotFound(err) {
			// If a service is not currently loaded, it will not appear in the list.
			// Ignore this as if the service has already been stopped
			return nil, nil
		}
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
	return []reconcile.Step{newUnitState(r.mon, r.name, activeStateActive)}, nil
}

type startUnit struct {
	mon  *unitMonitor
	name string
}

func newResetFailedUnit(mon *unitMonitor, name string) *resetFailedUnit {
	return &resetFailedUnit{
		mon:  mon,
		name: name,
	}
}

// Name returns the name of this service unit
func (r *resetFailedUnit) Name() string {
	return fmt.Sprintf("reset-status(%v)", r.name)
}

// Do resets the status of the underlying service unit if it's failed
func (r *resetFailedUnit) Do(ctx context.Context) ([]reconcile.Step, error) {
	unit, err := r.mon.status(r.name)
	if err != nil {
		if trace.IsNotFound(err) {
			// Ignore not loaded units
			return nil, nil
		}
		return nil, trace.Wrap(err, "failed to query service status").AddField("unit", r.name)
	}
	if unit.ActiveState != string(activeStateFailed) {
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

func newUnitState(mon *unitMonitor, name string, state serviceActiveState) *unitState {
	return &unitState{
		mon:   mon,
		name:  name,
		state: state,
	}
}

// Name returns the name of this service unit
func (r *unitState) Name() string {
	return fmt.Sprintf("unit-state(%v -> %v)", r.name, r.state)
}

// Do verifies whether the underlying unit is in the expected state
func (r *unitState) Do(context.Context) ([]reconcile.Step, error) {
	unit, err := r.mon.status(r.name)
	if err != nil {
		return nil, trace.Wrap(err, "failed to query service status").AddField("unit", r.name)
	}
	if unit.ActiveState == string(r.state) {
		return nil, nil
	}
	return nil, trace.CompareFailed("service in unexpected status").AddFields(map[string]interface{}{
		"unit":   r.name,
		"status": unit.ActiveState,
	})
}

type unitState struct {
	mon   *unitMonitor
	name  string
	state serviceActiveState
}

func verifyServiceState(mon *unitMonitor, state serviceActiveState, units []string, logger logrus.FieldLogger) (invalid []string, err error) {
	logger = logger.WithField("state", state)
	logger.Info("Verify services are in expected state.")
	unitsInState, err := mon.filterUnits(state)
	if err != nil {
		return nil, trace.Wrap(err, "failed to query service units")
	}
	existing := make(map[string]struct{}, len(units))
	for _, unit := range units {
		existing[unit] = struct{}{}
	}
	for _, unit := range unitsInState {
		delete(existing, unit.Name)
	}
	for unit := range existing {
		invalid = append(invalid, unit)
	}
	if len(invalid) == 0 {
		logger.Info("All services are in expected state.")
		return nil, nil
	}
	return invalid, trace.CompareFailed("some services are not in expected state").AddField("units", existing)
}

// verifyLeaderAddr ensures that the CoreDNS hosts configuration has the specified
// address as the current leader address.
// Returns an error in case of mismatch or any other error indicating that the file
// should be updated.
func verifyLeaderAddr(leaderAddr string) error {
	f, err := os.Open(constants.CoreDNSHosts)
	if err != nil {
		return trace.ConvertSystemError(err)
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Split(bufio.ScanLines)
	if !s.Scan() {
		err = s.Err()
		if err == nil {
			err = trace.NotFound("Coredns hosts configuration is empty")
		}
		return trace.Wrap(err, "failed to query Coredns hosts configuration file")
	}
	parts := strings.Fields(s.Text())
	if len(parts) != corednsNumFields {
		return trace.BadParameter("Coredns hosts configuration is invalid").AddField("out", s.Text())
	}
	if parts[0] == leaderAddr {
		return nil
	}
	return trace.CompareFailed("leader address does not match")
}

func writeLocalLeader(leaderAddr string) error {
	contents := fmt.Sprintln(leaderAddr, " ",
		constants.APIServerDNSName, " ",
		constants.APIServerDNSNameGravity, " ",
		constants.RegistryDNSName, " ",
		constants.LegacyAPIServerDNSName)
	return trace.ConvertSystemError(ioutil.WriteFile(
		constants.CoreDNSHosts, []byte(contents), constants.SharedFileMask))
}

// nodeUnitStateTarget describes a state target for a set of service units
// on a node
type nodeUnitStateTarget struct {
	stop  unitStateTarget
	start unitStateTarget
}

// unitStateTarget describes a state target for a set of service units
type unitStateTarget struct {
	units []string
	state serviceActiveState
}

type coordinateClient interface {
	AddVoter(ctx context.Context, key, addr string, term time.Duration)
	RemoveVoter(ctx context.Context, key, addr string, term time.Duration)
}

const corednsNumFields = 5

var (
	regularNodeUnitStateTarget = nodeUnitStateTarget{
		stop: unitStateTarget{
			units: controlServiceUnits,
			state: activeStateInactive,
		},
		start: unitStateTarget{
			units: serviceUnits,
			state: activeStateActive,
		},
	}

	leaderNodeUnitStateTarget = nodeUnitStateTarget{
		start: unitStateTarget{
			units: allServiceUnits,
			state: activeStateActive,
		},
		stop: unitStateTarget{
			state: activeStateInactive,
		},
	}

	allServiceUnits = append(serviceUnits, controlServiceUnits...)

	controlServiceUnits = []string{
		"kube-controller-manager.service",
		"kube-scheduler.service",
		"kube-apiserver.service",
	}

	serviceUnits = []string{
		"kube-kubelet.service",
		"kube-proxy.service",
		"docker.service",
		"registry.service",
		"coredns.service",
	}
)
