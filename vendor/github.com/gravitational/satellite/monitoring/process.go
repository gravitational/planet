/*
Copyright 2017 Gravitational, Inc.

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

package monitoring

import (
	"context"
	"fmt"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
	"github.com/gravitational/satellite/utils"

	"github.com/gravitational/trace"

	"github.com/mitchellh/go-ps"
)

// ProcessChecker validates that no conflicting processes are running
// and all required are
type ProcessChecker struct {
	// Conflicting contains list of processes which are not expected to run
	Conflicting []string
	// Required lists processes required to run
	Required []string
}

const processCheckerID = "process-checker"

// DefaultProcessChecker returns checker which will ensure no conflicting program is running
func DefaultProcessChecker() *ProcessChecker {
	return &ProcessChecker{
		Conflicting: []string{
			"dockerd",
			"lxd",
			"dnsmasq",
			"kube-apiserver",
			"kube-scheduler",
			"kube-controller-manager",
			"kube-proxy",
			"kubelet",
			"planet",
			"teleport",
		},
	}
}

// Name returns checker name
func (c *ProcessChecker) Name() string {
	return processCheckerID
}

// Check verifies that no conflicting process is running and all required are
func (c *ProcessChecker) Check(ctx context.Context, r health.Reporter) {
	running, err := ps.Processes()
	if err != nil {
		r.Add(NewProbeFromErr(processCheckerID, "failed to obtain running process list", trace.Wrap(err)))
		return
	}

	conflicting := utils.NewStringSet()
	required := utils.NewStringSetFromSlice(c.Required)
	for _, process := range running {
		if utils.StringInSlice(c.Conflicting, process.Executable()) {
			conflicting.Add(process.Executable())
		}
		if utils.StringInSlice(c.Required, process.Executable()) {
			required.Remove(process.Executable())
		}
	}

	if len(conflicting) == 0 && len(required) == 0 {
		r.Add(&pb.Probe{
			Checker: processCheckerID,
			Status:  pb.Probe_Running,
		})
		return
	}

	if len(conflicting) != 0 {
		r.Add(&pb.Probe{
			Checker: processCheckerID,
			Detail: fmt.Sprintf("potentially conflicting programs running: %v, note this is an issue only before Telekube is installed",
				conflicting.Slice()),
			Status: pb.Probe_Failed,
		})
	}

	if len(required) != 0 {
		r.Add(&pb.Probe{
			Checker: processCheckerID,
			Detail:  fmt.Sprintf("required processes not running: %q", required.Slice()),
			Status:  pb.Probe_Failed,
		})
	}
}
