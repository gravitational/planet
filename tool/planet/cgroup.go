/*
Copyright 2019 Gravitational, Inc.

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

/*
Cgroup Configuration
--------------------
Planet uses a customized cgroup structure, that's designed to prevent CPU resource starvation of critical services.

Notes:
- The cgroup customization is within planet only, the host system will not be configured
- Systems with less than 5 cores, will not reserve resources in kubernetes
- User tasks will be capped at a maximum CPU usage
  - 500 millicores on systems with less than 5 cores
  - 10% of system resources (1/10, 2/20,4/40 cores etc)
  - User tasks run with high scheduling priority
	- Ensures an administrator can always debug the system
	- However, because CPU usage is capped, an administrator shouldn't interfere with system services
- Planet services take scheduling precedence over kubernetes pods
  - kubernetes is responsible for inter-pod cgroup settings

*/
package main

import (
	"io/ioutil"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/davecgh/go-spew/spew"

	"github.com/containerd/cgroups"
	"github.com/gravitational/planet/lib/constants"
	"github.com/gravitational/planet/lib/utils"
	"github.com/gravitational/trace"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

type CgroupConfig struct {
	// Enabled indicates whether the planet container should apply the cgroup configuration.
	Enabled bool
	// Auto indicates whether planet is allowed to regenerate the Cgroup configuration. New versions of planet may embed
	// new rules.
	// If configuration is changed by a user, and they don't wish their settings to be overriden, auto should be set
	// to false.
	Auto bool

	// KubeReservedCpu is the amount of CPU to reserve within kubernetes for kubelet + docker (in millicores)
	KubeReservedCPU int
	// KubeSystemCPU is the amount of CPU to reserve within kubernetes for system services (in millicores)
	KubeSystemCPU int

	// Cgroups is a list of cgroup configurations to apply within the planet container
	Cgroups []CgroupEntry
}

type CgroupEntry struct {
	specs.LinuxResources

	// Path is the cgroup hierarchy path this setting applies to
	Path []string `json:"path"`
}

// updateCgroups updates the cgroups as per configuration
func upsertCgroups(isMaster bool) error {
	log := logrus.WithField(trace.Component, "cgroup")
	log.WithField("master", isMaster).Info("Upsert cgroup configuration")

	configPath := path.Join(StateDir, "planet-cgroups.conf")
	config, err := readCgroupConfig(configPath)
	if err != nil && !trace.IsNotFound(err) {
		return trace.Wrap(err)
	}
	if trace.IsNotFound(err) {
		config = defaultCgroupConfig(runtime.NumCPU(), isMaster)
	}
	if config.Enabled && config.Auto {
		// if the configuration is automatically generated, regenerate it each start to pick up any changes
		// with new planet releases
		config = defaultCgroupConfig(runtime.NumCPU(), isMaster)
	}

	log.Info(spew.Sdump(config))

	if !config.Enabled {
		return nil
	}

	// rewrite configuration (it may have been updated with new defaults)
	err = writeCgroupConfig(configPath, config)
	if err != nil {
		return trace.Wrap(err)
	}
	var errors []error

	// try and set the cgroups
	for _, entry := range config.Cgroups {
		if len(entry.Path) == 0 {
			return trace.BadParameter("cgroup spec with no path set: %v", entry)
		}

		switch {
		case strings.HasSuffix(entry.Path[0], ".slice"):
			errors = append(errors, trace.Wrap(upsertSystemd(entry)))
		default:
			errors = append(errors, trace.Wrap(upsertCgroupV1(entry)))
		}
	}

	return trace.NewAggregate(errors...)
}

func upsertSystemd(entry CgroupEntry) error {
	_, err := cgroups.New(cgroups.Systemd, systemdPath(entry.Path...), &entry.LinuxResources)
	return trace.Wrap(err)
}

func systemdPath(s ...string) cgroups.Path {
	return func(subsystem cgroups.Name) (string, error) {
		return filepath.Join(s...), nil
	}
}

func upsertCgroupV1(entry CgroupEntry) error {
	_, err := cgroups.New(cgroups.V1, cgroups.StaticPath(path.Join(entry.Path...)), &entry.LinuxResources)
	return trace.Wrap(err)
}

// DefaultCgroupCPUPeriod in us (100000us = 100ms)
const DefaultCgroupCPUPeriod = 100000

func defaultCgroupConfig(numCPU int, isMaster bool) *CgroupConfig {
	// calculate 10% of system for user cgroups
	totalQuota := numCPU * DefaultCgroupCPUPeriod
	userQuota := totalQuota / 10
	// minimum of 1/2 core for quota
	if userQuota < DefaultCgroupCPUPeriod/2 {
		userQuota = DefaultCgroupCPUPeriod / 2
	}
	userQuotaI := int64(userQuota)
	periodI := uint64(DefaultCgroupCPUPeriod)

	swappiness := uint64(20)
	shares100 := uint64(100)
	shares2 := uint64(2)

	config := CgroupConfig{
		Enabled: true,
		Auto:    true,
		Cgroups: []CgroupEntry{
			CgroupEntry{
				Path: []string{"user"},
				LinuxResources: specs.LinuxResources{
					CPU: &specs.LinuxCPU{
						Shares: &shares100,
						Quota:  &userQuotaI,
						Period: &periodI,
					},
				},
			},
			CgroupEntry{
				Path: []string{"system.slice"},
				LinuxResources: specs.LinuxResources{
					CPU: &specs.LinuxCPU{
						Shares: &shares100,
					},
				},
			},
			CgroupEntry{
				Path: []string{"kube-pods"},
				LinuxResources: specs.LinuxResources{
					CPU: &specs.LinuxCPU{
						Shares: &shares2,
					},
					Memory: &specs.LinuxMemory{
						Swappiness: &swappiness,
					},
				},
			},
		},
	}

	// if the system has limited CPU power, we only set the cgroup hierarchy
	// and don't set kube-reserved
	if numCPU <= 4 {
		return &config
	}

	// The amount of resources to reserve for kubelet + docker on a busy system
	// Reference: http://node-perf-dash.k8s.io/#/builds
	nodeReservedMilli := 800
	if isMaster {
		// reserve 1 additional core when master.
		//
		nodeReservedMilli = 1800
	}
	config.KubeReservedCPU = nodeReservedMilli

	// 1 CPU = 1000 millicores
	totalMillis := numCPU * 1000
	// System reserved - 10% of cpu + 200 millicores
	config.KubeSystemCPU = (totalMillis / 10) + 200

	return &config
}

func readCgroupConfig(path string) (*CgroupConfig, error) {
	buf, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, trace.ConvertSystemError(err)
	}

	var config CgroupConfig

	err = yaml.Unmarshal(buf, &config)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &config, nil
}

func writeCgroupConfig(path string, config *CgroupConfig) error {
	buf, err := yaml.Marshal(config)
	if err != nil {
		return trace.Wrap(err)
	}

	return trace.Wrap(utils.SafeWriteFile(path, buf, constants.SharedReadMask))
}
