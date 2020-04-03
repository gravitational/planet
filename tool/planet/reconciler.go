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

// Name returns the name of this plan.
// Implements reconciler.Plan
func (r *agentPlan) Name() string {
	return "agent state reconciler"
}

// Creates a new plan for agent operation
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
	if err := verifyLeaderAddr(r.leaderAddr); err != nil {
		logger.WithError(err).Warn("Failed to check local leader DNS configuration, will update configuration.")
		steps = append(steps, updateDNS{leaderAddr: r.leaderAddr})
	}
	state := activeStateActive
	units := serviceUnits
	if r.localAddr != r.leaderAddr {
		state = activeStateInactive
		units = controlServiceUnits
	}
	invalid, err := verifyServiceState(r.mon, state, units, logger)
	if err == nil {
		return steps, nil
	}
	logger.WithError(err).Warn("Failed to verify service state, will reconcile.")
	if r.localAddr == r.leaderAddr {
		logger.Debug("Start service units.")
		return append(steps, startUnits(r.mon, invalid...)...), nil
	}
	logger.Debug("Stop service units.")
	return append(steps, stopUnits(r.mon, invalid...)...), nil
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
	client         *leader.Client
	mon            *unitMonitor
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
		unitState: unitState{
			mon:   mon,
			name:  name,
			state: activeStateInactive,
		},
	}
}

// Name returns the name of this service unit
func (r *resetFailedUnit) Name() string {
	return fmt.Sprintf("reset-status(%v)", r.name)
}

// Do resets the status of the underlying service unit if it's failed
func (r *resetFailedUnit) Do(ctx context.Context) ([]reconcile.Step, error) {
	steps, err := r.unitState.Do(ctx)
	if err != nil {
		if trace.IsNotFound(err) {
			// If a service is not currently loaded, it will not appear in the list.
			// Ignore this as if the service has already been stopped
			return nil, nil
		}
		return nil, trace.Wrap(err)
	}
	if err := r.mon.resetStatus(r.name); err != nil {
		return nil, trace.Wrap(err, "failed to reset service status").AddField("unit", r.name)
	}
	return steps, nil
}

type resetFailedUnit struct {
	unitState
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

const corednsNumFields = 5

var (
	serviceUnits = append([]string{
		"kube-kubelet.service",
		"kube-proxy.service",
		"docker.service",
		"registry.service",
		"coredns.service",
	}, controlServiceUnits...)

	controlServiceUnits = []string{
		"kube-controller-manager.service",
		"kube-scheduler.service",
		"kube-apiserver.service",
	}
)
