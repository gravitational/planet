package monitoring

import (
	"github.com/gravitational/satellite/agent"
	"github.com/gravitational/satellite/monitoring"
	"github.com/gravitational/satellite/monitoring/collector"
	"github.com/gravitational/trace"
	"github.com/prometheus/client_golang/prometheus"
)

// AddMetrics exposes specific metrics to prometheus
func AddMetrics(config *Config) error {
	etcdConfig := &monitoring.ETCDConfig{
		Endpoints: config.ETCDConfig.Endpoints,
		CAFile:    config.ETCDConfig.CAFile,
		CertFile:  config.ETCDConfig.CertFile,
		KeyFile:   config.ETCDConfig.KeyFile,
	}

	var metrics []prometheus.Collector
	var err error
	switch config.Role {
	case agent.RoleMaster:
		metrics, err = addMetricsToMaster(config.KubeAddr, etcdConfig)
	case agent.RoleNode:
		metrics, err = addMetricsToNode(config.KubeAddr, etcdConfig)
	}
	if err != nil {
		return trace.Wrap(err)
	}

	for _, metric := range metrics {
		if err = prometheus.Register(metric); err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}

func addMetricsToMaster(kubeAddr string, etcdConfig *monitoring.ETCDConfig) (metrics []prometheus.Collector, err error) {
	collector, err := collector.NewMetricsCollector(etcdConfig, kubeAddr, agent.RoleMaster)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	metrics = append(metrics, collector)
	return metrics, nil
}

func addMetricsToNode(kubeAddr string, etcdConfig *monitoring.ETCDConfig) (metrics []prometheus.Collector, err error) {
	collector, err := collector.NewMetricsCollector(etcdConfig, kubeAddr, agent.RoleNode)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	metrics = append(metrics, collector)
	return metrics, nil
}
