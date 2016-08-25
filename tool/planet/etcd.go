package main

import (
	"os"
	"os/exec"

	log "github.com/Sirupsen/logrus"
	"github.com/gravitational/planet/lib/box"
	"github.com/gravitational/trace"
)

func etcdPromote(name, initialCluster, initialClusterState string) error {
	env, err := box.ReadEnvironment(ContainerEnvironmentFile)
	if err != nil {
		return trace.Wrap(err)
	}

	if env.Get(EnvEtcdProxy) == EtcdProxyOff {
		log.Infof("etcd is not running in proxy mode, nothing to do")
		return nil
	}

	newEnv := map[string]string{
		EnvEtcdProxy:               EtcdProxyOff,
		EnvEtcdMemberName:          name,
		EnvEtcdInitialCluster:      initialCluster,
		EnvEtcdInitialClusterState: initialClusterState,
	}

	log.Infof("updating etcd environment: %v", newEnv)
	for k, v := range newEnv {
		env.Upsert(k, v)
	}

	if err := box.WriteEnvironment(ContainerEnvironmentFile, env); err != nil {
		return trace.Wrap(err)
	}

	log.Infof("stopping etcd proxy")
	if err := exec.Command("/bin/systemctl", "stop", "etcd").Run(); err != nil {
		return trace.Wrap(err)
	}

	log.Infof("removing %v", ETCDProxyDir)
	if err := os.RemoveAll(ETCDProxyDir); err != nil {
		return trace.Wrap(err)
	}

	log.Infof("starting etcd")
	if err := exec.Command("/bin/systemctl", "start", "etcd").Run(); err != nil {
		return trace.Wrap(err)
	}

	return nil
}
