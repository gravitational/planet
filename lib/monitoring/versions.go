/*
Copyright 2018 Gravitational, Inc.

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
	"os/exec"

	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
)

// NewVersionCollector returns new instance of version collector probe
func NewVersionCollector() *VersionCollector {
	return &VersionCollector{}
}

// VersionCollector is a special type of probe that collects
// and reports versions of the internal components of planet
type VersionCollector struct {
}

// Name returns name of this collector
func (r *VersionCollector) Name() string { return "versions" }

// Check collects versions of all components and adds information to reporter
func (r *VersionCollector) Check(ctx context.Context, reporter health.Reporter) {
	for _, checker := range infoCheckers {
		output, err := exec.Command(checker.command[0], checker.command[1:]...).CombinedOutput()
		out := string(output)
		if err != nil {
			out += err.Error()
		}
		reporter.Add(&pb.Probe{
			Checker: checker.component,
			Detail:  string(output),
			Status:  pb.Probe_Running,
		})
	}
}

type infoChecker struct {
	command   []string
	component string
}

var infoCheckers = []infoChecker{
	{command: []string{"/bin/uname", "-a"}, component: "system-version"},
	{command: []string{"/bin/systemd", "--version"}, component: "systemd-version"},
	{command: []string{"/usr/bin/docker", "info"}, component: "docker-version"},
	{command: []string{"/usr/bin/etcd", "--version"}, component: "etcd-version"},
	{command: []string{"/usr/bin/kubelet", "--version"}, component: "kubelet-version"},
	{command: []string{"/usr/bin/coredns", "-version"}, component: "coredns-version"},
	{command: []string{"/usr/bin/dbus-daemon", "--version"}, component: "dbus-version"},
	{command: []string{"/usr/bin/serf", "--version"}, component: "serf-version"},
	{command: []string{"/usr/bin/flanneld", "--version"}, component: "flanneld-version"},
	{command: []string{"/usr/bin/registry", "--version"}, component: "registry-version"},
	{command: []string{"/usr/bin/helm", "version", "--client"}, component: "helm-client-version"},
}
