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
	"github.com/gravitational/satellite/agent"
	"github.com/gravitational/satellite/monitoring"
	"github.com/gravitational/satellite/monitoring/collector"
	"github.com/gravitational/trace"
	"github.com/prometheus/client_golang/prometheus"
)

// AddMetrics exposes specific metrics to Prometheus
func AddMetrics(node agent.Agent, config *Config) error {
	etcdConfig := &monitoring.ETCDConfig{
		Endpoints: config.ETCDConfig.Endpoints,
		CAFile:    config.ETCDConfig.CAFile,
		CertFile:  config.ETCDConfig.CertFile,
		KeyFile:   config.ETCDConfig.KeyFile,
	}

	var mc *collector.MetricsCollector
	var err error

	switch config.Role {
	case agent.RoleMaster:
		mc, err = collector.NewMetricsCollector(etcdConfig, config.KubeAddr, agent.RoleMaster)
	case agent.RoleNode:
		mc, err = collector.NewMetricsCollector(etcdConfig, config.KubeAddr, agent.RoleNode)
	}
	if err = prometheus.Register(mc); err != nil {
		return trace.Wrap(err)
	}
	return nil
}