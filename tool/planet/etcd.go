package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	etcd "github.com/coreos/etcd/client"
	etcdconf "github.com/gravitational/coordinate/config"
	"github.com/gravitational/planet/lib/box"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

// etcdPromote promotes running etcd proxy to a full member; does nothing if it's already
// running in proxy mode.
//
// Parameters name, initial cluster and state are ones produced by the 'member add'
// command.
//
// Whether etcd is running in proxy mode is determined by ETCD_PROXY environment variable
// normally set in /etc/container-environment inside planet.
//
// To promote proxy to a member we update ETCD_PROXY to disable proxy mode, wipe out
// its state directory and restart the service, as suggested by etcd docs.
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

	out, err := exec.Command("/bin/systemctl", "stop", "etcd").CombinedOutput()
	log.Infof("stopping etcd: %v", string(out))
	if err != nil {
		return trace.Wrap(err, fmt.Sprintf("failed to stop etcd: %v", string(out)))
	}

	log.Infof("removing %v", ETCDProxyDir)
	if err := os.RemoveAll(ETCDProxyDir); err != nil {
		return trace.Wrap(err)
	}

	out, err = exec.Command("/bin/systemctl", "start", ETCDServiceName).CombinedOutput()
	log.Infof("starting etcd: %v", string(out))
	if err != nil {
		return trace.Wrap(err, fmt.Sprintf("failed to start etcd: %v", string(out)))
	}

	if env.Get(EnvRole) == PlanetRoleMaster {
		out, err = exec.Command("/bin/systemctl", "start", APIServerServiceName).CombinedOutput()
		log.Infof("starting kube-apiserver: %v", string(out))
		if err != nil {
			return trace.Wrap(err, fmt.Sprintf("failed to start kube-apiserver: %v", string(out)))
		}
	}

	return nil
}

func etcdBackup(config etcdconf.Config, file string, prefix string) error {
	err := config.CheckAndSetDefaults()
	if err != nil {
		return trace.Wrap(err)
	}

	client, err := config.NewClient()
	if err != nil {
		return trace.Wrap(err)
	}

	kapi := etcd.NewKeysAPI(client)
	ctx := context.WithTimeout(context.Background(), ETCDBackupTimeout)

	// This retrieves the entire etcd datastore after prefix into a go object, which could be fairly large
	// so we may need to evaluate changing the approach if we have some large etcd datastores in the wild
	res, err := kapi.Get(ctx, prefix, &etcd.GetOptions{Sort: true, Recursive: true})
	if err != nil {
		return err
	}

	f, err := os.Create("/tmp/dat2")
	if err != nil {
		return trace.Wrap(err)
	}
	enc := json.NewEncoder(f)
	err := enc.Encode(&res)
	if err != nil {
		return trace.Wrap(err)
	}
}

func etcdRestore(config etcdconf.Config, file string, prefix string) error {

}

func convertError(err error) error {
	if err == nil {
		return nil
	}
	switch err := err.(type) {
	case *etcd.ClusterError:
		return trace.Wrap(err, err.Detail())
	case etcd.Error:
		switch err.Code {
		case etcd.ErrorCodeKeyNotFound:
			return trace.NotFound(err.Error())
		case etcd.ErrorCodeNotFile:
			return trace.BadParameter(err.Error())
		case etcd.ErrorCodeNodeExist:
			return trace.AlreadyExists(err.Error())
		case etcd.ErrorCodeTestFailed:
			return trace.CompareFailed(err.Error())
		}
	}
	return err
}
