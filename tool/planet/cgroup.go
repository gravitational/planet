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
- The cgroup customization is within planet cgroup namespace only
- Systems with less than 5 cores, will not reserve resources in kubernetes
  - Relative prioritization will still be applied
- User tasks will be capped at a maximum CPU usage
  - 500 millicores on systems with less than 5 cores
  - 10% of system resources (0.6/6, 1/10, 2/20, 4/40 cores etc) on 6 cores or more
  - User tasks run with high scheduling priority
	- The idea is, an administrator should always be able to troubleshoot a system
	- However, because CPU usage is capped at 10%, an administrator shouldn't interfere with system services
- Planet services and user tasks take scheduling priority over kubernetes pods
  - System and User tasks always have priority over pods
  - kubernetes remains responsible for setting pod cgroup settings, and relative priority between pods
*/
package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"path"
	"runtime"
	"strings"
	"text/template"

	systemdDbus "github.com/coreos/go-systemd/dbus"
	"github.com/davecgh/go-spew/spew"
	"github.com/godbus/dbus"

	"github.com/containerd/cgroups"
	"github.com/gravitational/planet/lib/box"
	"github.com/gravitational/planet/lib/constants"
	"github.com/gravitational/planet/lib/utils"
	"github.com/gravitational/trace"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	yaml "gopkg.in/yaml.v2"
)

type CgroupConfig struct {
	// Enabled indicates whether the planet container should apply the cgroup configuration.
	Enabled bool
	// Auto indicates whether planet is allowed to regenerate the Cgroup configuration.
	// If a new version of planet embeds new rules, this allows planet to update the configuration file. If the
	// config is externally managed or updated by a user, set this to false so planet doesn't overwrite the settings.
	Auto bool

	// KubeReservedCPUMillicores is the amount of CPU to reserve within kubernetes for kubelet + docker (in millicores)
	KubeReservedCPUMillicores int
	// KubeSystemCPUMillicores is the amount of CPU to reserve within kubernetes for system services (in millicores)
	KubeSystemCPUMillicores int

	// Cgroups is a list of cgroup configurations to apply within the planet container
	Cgroups []CgroupEntry
}

type CgroupEntry struct {
	specs.LinuxResources

	// Path is the cgroup hierarchy path this setting applies to
	Path string
}

// upsertCgroups reads/updates the cgroup configuration and applies it to the system
func upsertCgroups(isMaster bool) error {
	log := logrus.WithField(trace.Component, "cgroup")
	log.WithField("master", isMaster).Info("Upsert cgroup configuration")

	// Cgroup namespaces aren't currently available in redhat/centos based kernels
	// only enable resource starvation prevention on kernels that have cgroup namespaces that is needed to enact our
	// cgroup hierarchy
	cgroupsEnabled, err := box.CgroupNSEnabled()
	if err != nil {
		return trace.Wrap(err)
	}
	if !cgroupsEnabled {
		log.Warn("Cgroup namespaces aren't enabled in the kernel, disabling resource starvation prevention.")
		return nil
	}

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

	log.Info("cgroup configuration: ", spew.Sdump(config))

	if !config.Enabled {
		return nil
	}

	// rewrite configuration on disk (it may have been updated with new defaults)
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
		case strings.HasSuffix(entry.Path, ".slice"):
			errors = append(errors, trace.Wrap(upsertSystemd(entry)))
		default:
			errors = append(errors, trace.Wrap(upsertCgroupV1(entry)))
		}
	}

	errors = append(errors, trace.Wrap(writeKubeReservedEnvironment(config)))

	return trace.NewAggregate(errors...)
}

func upsertSystemd(entry CgroupEntry) error {
	// use dbus to set systemd unit properties
	conn, err := systemdDbus.New()
	if err != nil {
		return trace.Wrap(err)
	}
	defer conn.Close()

	properties := []systemdDbus.Property{
		newProperty("MemoryAccounting", true),
		newProperty("CPUAccounting", true),
		newProperty("BlockIOAccounting", true),
	}

	if entry.CPU.Shares != nil {
		properties = append(properties, newProperty("CPUShares", entry.CPU.Shares))
	}

	return trace.Wrap(conn.SetUnitProperties(entry.Path, true, properties...))
}

func newProperty(name string, units interface{}) systemdDbus.Property {
	return systemdDbus.Property{
		Name:  name,
		Value: dbus.MakeVariant(units),
	}
}

func upsertCgroupV1(entry CgroupEntry) error {
	_, err := cgroups.New(cgroups.V1, cgroups.StaticPath(entry.Path), &entry.LinuxResources)
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

	config := CgroupConfig{
		Enabled: true,
		Auto:    true,
		Cgroups: []CgroupEntry{
			// /user
			// - cgroup for user processes, capped cpu usage
			CgroupEntry{
				Path: "user",
				LinuxResources: specs.LinuxResources{
					CPU: &specs.LinuxCPU{
						Shares: u64(1024),
						Quota:  i64(int64(userQuota)),
						Period: u64(DefaultCgroupCPUPeriod),
					},
				},
			},
			// /system.slice
			// - cgroup for planet services
			// - set swapinness to 0
			CgroupEntry{
				Path: "system.slice",
				LinuxResources: specs.LinuxResources{
					CPU: &specs.LinuxCPU{
						Shares: u64(1024),
					},
					Memory: &specs.LinuxMemory{
						Swappiness: u64(0),
					},
				},
			},
			// /kube-pods
			// - cgroup for kubernetes pods
			// - minimum cpu scheduling priority
			// - Set swappiness to 20, so processes are less likely to swap. (Kubernetes recommends no swap)
			CgroupEntry{
				Path: "kube-pods",
				LinuxResources: specs.LinuxResources{
					CPU: &specs.LinuxCPU{
						Shares: u64(2),
					},
					Memory: &specs.LinuxMemory{
						Swappiness: u64(20),
					},
				},
			},
		},
	}

	// if the system has limited CPU power, we only set the cgroup hierarchy
	// and don't set kube-reserved / system reserved
	if numCPU <= 4 {
		return &config
	}

	// The amount of resources to reserve for kubelet + docker on a busy system
	// Reference: http://node-perf-dash.k8s.io/#/builds
	nodeReservedMilli := 800
	if isMaster {
		// reserve 1 additional core when master.
		// this is just an educated guess at this point
		nodeReservedMilli = 1800
	}
	config.KubeReservedCPUMillicores = nodeReservedMilli

	// 1 CPU = 1000 millicores
	totalMillis := numCPU * 1000

	// System reserved
	// - 10% of cpu for admin tasks (pods will be able to burst into this CPU time most often)
	// - 200 millicores (serf/coredns/satellite/systemd etc services)
	config.KubeSystemCPUMillicores = (totalMillis / 10) + 200

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

func writeKubeReservedEnvironment(config *CgroupConfig) error {
	env := make(map[string]string)
	if config.KubeReservedCPUMillicores > 0 {
		env["KUBE_RESERVED"] = fmt.Sprintf("cpu=%vm", config.KubeReservedCPUMillicores)
	}
	if config.KubeSystemCPUMillicores > 0 {
		env["KUBE_SYSTEM_RESERVED"] = fmt.Sprintf("cpu=%vm", config.KubeSystemCPUMillicores)
	}

	var b bytes.Buffer
	err := kubeReservedEnv.Execute(&b, &env)
	if err != nil {
		return trace.Wrap(err)
	}

	return trace.Wrap(utils.SafeWriteFile("/run/kubernetes-reserved.env", b.Bytes(), constants.SharedReadMask))
}

var kubeReservedEnv = template.Must(
	template.New("kube-reserved-env").Parse(`{{ range $key, $value := . }}{{ $key }}="{{ $value }}"
{{ end }}
`))

func u64(n uint64) *uint64 {
	return &n
}

func i64(n int64) *int64 {
	return &n
}
