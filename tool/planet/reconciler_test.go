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
	"context"
	"time"

	"github.com/gravitational/planet/lib/reconcile"

	"github.com/gravitational/trace"
	"github.com/sirupsen/logrus"
	check "gopkg.in/check.v1"
)

type S struct{}

var _ = check.Suite(&S{})

func (*S) TestAgentReconciler(c *check.C) {
	var testCases = []struct {
		comment  string
		plan     agentPlan
		expected []reconcile.Step
	}{
		{
			comment: "leader with all services stopped",
			plan: agentPlan{
				localAddr:        "addr",
				leaderAddr:       "addr",
				client:           testClient,
				verifyLeaderAddr: testVerifyLeaderAddr("addr"),
				verifyServiceState: testVerifyServiceState(nodeState{
					stopped: allServiceUnits,
				}),
			},
			expected: startedUnits(allServiceUnits...),
		},
		{
			comment: "regular node with all services stopped",
			plan: agentPlan{
				localAddr:        "addr",
				leaderAddr:       "addr2",
				client:           testClient,
				verifyLeaderAddr: testVerifyLeaderAddr("addr2"),
				verifyServiceState: testVerifyServiceState(nodeState{
					stopped: allServiceUnits,
				}),
			},
			expected: startedUnits(serviceUnits...),
		},
		{
			comment: "node after losing leadership",
			plan: agentPlan{
				localAddr:        "addr",
				leaderAddr:       "addr2",
				client:           testClient,
				verifyLeaderAddr: testVerifyLeaderAddr("addr2"),
				verifyServiceState: testVerifyServiceState(nodeState{
					running: allServiceUnits,
				}),
			},
			expected: stoppedUnits(controlServiceUnits...),
		},
		{
			comment: "node after becoming leader",
			plan: agentPlan{
				localAddr:        "addr",
				leaderAddr:       "addr",
				client:           testClient,
				verifyLeaderAddr: testVerifyLeaderAddr("addr"),
				verifyServiceState: testVerifyServiceState(nodeState{
					running: serviceUnits,
					stopped: controlServiceUnits,
				}),
			},
			expected: startedUnits(controlServiceUnits...),
		},
		{
			comment: "restart a stopped service",
			plan: agentPlan{
				localAddr:        "addr",
				leaderAddr:       "addr",
				client:           testClient,
				verifyLeaderAddr: testVerifyLeaderAddr("addr"),
				verifyServiceState: testVerifyServiceState(nodeState{
					running: sliceSubtract(allServiceUnits, "kube-kubelet.service"),
					stopped: []string{"kube-kubelet.service"},
				}),
			},
			expected: startedUnits("kube-kubelet.service"),
		},
		{
			comment: "reset leader DNS address",
			plan: agentPlan{
				localAddr:        "addr",
				leaderAddr:       "addr2",
				client:           testClient,
				verifyLeaderAddr: testVerifyLeaderAddr("addr"),
				verifyServiceState: testVerifyServiceState(nodeState{
					running: serviceUnits,
					stopped: controlServiceUnits,
				}),
			},
			expected: []reconcile.Step{
				updateDNS{leaderAddr: "addr2"},
			},
		},
	}
	for _, tc := range testCases {
		comment := check.Commentf(tc.comment)
		steps, err := tc.plan.Create(context.TODO())
		c.Assert(err, check.IsNil, comment)
		c.Assert(steps, check.DeepEquals, tc.expected, comment)
	}
}

var testClient dummyClient

func (dummyClient) AddVoter(context.Context, string, string, time.Duration)    {}
func (dummyClient) RemoveVoter(context.Context, string, string, time.Duration) {}

type dummyClient struct{}

func testVerifyLeaderAddr(leaderAddr string) func(string) error {
	return func(addr string) error {
		if leaderAddr == addr {
			return nil
		}
		return trace.BadParameter("leader has changed")
	}
}

func testVerifyServiceState(nodeState nodeState) func(*unitMonitor, serviceActiveState, []string, logrus.FieldLogger) ([]string, error) {
	return func(_ *unitMonitor, state serviceActiveState, units []string, _ logrus.FieldLogger) ([]string, error) {
		if state == activeStateInactive {
			unitsToStop := sliceSubtract(units, nodeState.stopped...)
			if len(unitsToStop) != 0 {
				return unitsToStop, trace.BadParameter("services need to be stopped")
			}
			return nil, nil
		}
		unitsToStart := sliceSubtract(units, nodeState.running...)
		if len(unitsToStart) != 0 {
			return unitsToStart, trace.BadParameter("services need to be started")
		}
		return nil, nil
	}
}

type nodeState struct {
	stopped []string
	running []string
}

func sliceSubtract(stack []string, remove ...string) (result []string) {
	result = make([]string, 0, len(stack))
L:
	for _, s := range stack {
		for _, r := range remove {
			if s == r {
				continue L
			}
		}
		result = append(result, s)
	}
	return result
}

func startedUnits(units ...string) (result []reconcile.Step) {
	for _, unit := range units {
		result = append(result, newStartUnit(nil, unit))
	}
	return result
}

func stoppedUnits(units ...string) (result []reconcile.Step) {
	for _, unit := range units {
		result = append(result, newStopUnit(nil, unit))
	}
	return result
}
