package monitoring

import (
	"github.com/gravitational/planet/lib/agent"
	"github.com/gravitational/planet/lib/agent/health"
)

type Config struct {
	Role     Role
	KubeAddr string
}

type Role string

const (
	RoleMaster Role = "master"
	RoleNode        = "node"
)

func AddCheckers(agent agent.Agent, conf *Config) {
	switch conf.Role {
	case RoleMaster:
		agent.AddChecker(kubeApiServerHealth())
		agent.AddChecker(componentStatusHealth(conf.KubeAddr))
		agent.AddChecker(dockerHealth())
		agent.AddChecker(dockerRegistryHealth())
		agent.AddChecker(etcdHealth())
		agent.AddChecker(systemdHealth())
	case RoleNode:
		agent.AddChecker(kubeletHealth())
		agent.AddChecker(dockerHealth())
		agent.AddChecker(etcdServiceHealth(conf.KubeAddr))
		agent.AddChecker(systemdHealth())
	}
}

func kubeApiServerHealth() health.Checker {
	return newChecker(newHTTPHealthzChecker("http://127.0.0.1:8080/healthz", kubeHealthz), "kube-apiserver")
}

func kubeletHealth() health.Checker {
	return newChecker(newHTTPHealthzChecker("http://127.0.0.1:10248/healthz", kubeHealthz), "kubelet")
}

func componentStatusHealth(kubeAddr string) health.Checker {
	return newChecker(&componentStatusChecker{hostPort: kubeAddr}, "componentstatuses")
}

func etcdHealth() health.Checker {
	return newChecker(newHTTPHealthzChecker("http://127.0.0.1:2379/health", etcdChecker), "etcd-healthz")
}

func etcdServiceHealth(kubeAddr string) health.Checker {
	return newChecker(&kubeChecker{hostPort: kubeAddr, checkerFunc: etcdKubeServiceChecker}, "etcd-service")
}

func dockerHealth() health.Checker {
	return newChecker(newUnixSocketHealthzChecker("http://docker/version", "/var/run/docker.sock",
		dockerChecker), "docker")
}

func dockerRegistryHealth() health.Checker {
	return newChecker(newHTTPHealthzChecker("http://127.0.0.1:5000/v2/", dockerChecker), "docker-registry")
}

func systemdHealth() health.Checker {
	return newChecker(systemdChecker{}, "systemd")
}
