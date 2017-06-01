package monitoring

import (
	"github.com/gravitational/satellite/agent"
	"github.com/gravitational/satellite/monitoring"
	"github.com/gravitational/satellite/monitoring/etcd"
	"github.com/gravitational/trace"
	"github.com/prometheus/client_golang/prometheus"
)

// AddMetrics add exporters for exposing data with Prometheus
func AddMetrics(config *Config) error {
	etcdConfig := &monitoring.ETCDConfig{
		Endpoints: config.ETCDConfig.Endpoints,
		CAFile:    config.ETCDConfig.CAFile,
		CertFile:  config.ETCDConfig.CertFile,
		KeyFile:   config.ETCDConfig.KeyFile,
	}
	switch config.Role {
	case agent.RoleMaster:
		mc, err := addMetricsToMaster(etcdConfig)
		if err != nil {
			return trace.Wrap(err)
		}
		prometheus.MustRegister(mc...)
	case agent.RoleNode:
		mc, err := addMetricsToNode(etcdConfig)
		if err != nil {
			return trace.Wrap(err)
		}
		prometheus.MustRegister(mc...)
	}
	return nil
}

func addMetricsToMaster(etcdConfig *monitoring.ETCDConfig) ([]prometheus.Collector, error) {
	var mc []prometheus.Collector

	// ETCD stats collector
	etcdExporter, err := etcd.NewExporter(etcdConfig)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	mc = append(mc, etcdExporter)
	return mc, nil
}

func addMetricsToNode(etcdConfig *monitoring.ETCDConfig) ([]prometheus.Collector, error) {
	var mc []prometheus.Collector

	// ETCD stats collector
	etcdExporter, err := etcd.NewExporter(etcdConfig)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	mc = append(mc, etcdExporter)
	return mc, nil
}
