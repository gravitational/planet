package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	etcd "github.com/coreos/etcd/client"
	"github.com/coreos/go-systemd/dbus"
	"github.com/davecgh/go-spew/spew"
	etcdconf "github.com/gravitational/coordinate/config"
	backup "github.com/gravitational/etcd-backup/lib/etcd"
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

func etcdBackup(backupFile string) error {
	ctx := context.TODO()
	// If a backup from a previous upgrade exists, clean it up
	if _, err := os.Stat(backupFile); err == nil {
		err = os.Remove(backupFile)
		if err != nil {
			return trace.Wrap(err)
		}
	}

	backupConf := backup.BackupConfig{
		EtcdConfig: etcdconf.Config{
			Endpoints: []string{DefaultEtcdEndpoints},
			KeyFile:   DefaultEtcdctlKeyFile,
			CertFile:  DefaultEtcdctlCertFile,
			CAFile:    DefaultEtcdctlCAFile,
		},
		Prefix: []string{"/"}, // Backup all etcd data
		File:   backupFile,
		Log:    log.StandardLogger(),
	}
	log.Info("BackupConfig: ", spew.Sdump(backupConf))

	err := backup.Backup(ctx, backupConf)
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}

func etcdRestore() error {
	return nil
}

// etcdDisable disabled etcd on this machine
// Used during upgrades
func etcdDisable() error {
	conn, err := dbus.New()
	if err != nil {
		return trace.Wrap(err)
	}

	changes, err := conn.MaskUnitFiles([]string{ETCDServiceName}, false, true)
	if err != nil {
		return trace.Wrap(err)
	}
	for _, change := range changes {
		log.Debugf("unmask: type: %v filename: %v destination: %v", change.Type, change.Filename, change.Destination)
	}

	c := make(chan string)
	_, err = conn.StopUnit(ETCDServiceName, "replace", c)
	if err != nil {
		return trace.Wrap(err)
	}

	status := <-c
	if strings.ToLower(status) != "done" {
		return trace.BadParameter("Systemd stop unit recieved unexpected result: %v", status)
	}
	return nil
}

// etcdEnable enables a disabled etcd node
func etcdEnable() error {
	conn, err := dbus.New()
	if err != nil {
		return trace.Wrap(err)
	}

	changes, err := conn.UnmaskUnitFiles([]string{ETCDServiceName}, false)
	if err != nil {
		return trace.Wrap(err)
	}
	for _, change := range changes {
		log.Debugf("unmask: type: %v filename: %v destination: %v", change.Type, change.Filename, change.Destination)
	}

	c := make(chan string)
	//https://godoc.org/github.com/coreos/go-systemd/dbus#Conn.StartUnit
	_, err = conn.StartUnit(ETCDServiceName, "replace", c)
	if err != nil {
		return trace.Wrap(err)
	}

	status := <-c
	if strings.ToLower(status) != "done" {
		return trace.BadParameter("Systemd stop unit recieved unexpected result: %v", status)
	}
	return nil
}

// etcdUpgradeMaster upgrades the first server in the cluster
func etcdUpgradeMaster(file string) error {
	return nil
}

// etcdUpgradeSlave upgrades additional masters or proxies in etcd
func etcdUpgradeSlave() error {
	return nil
}

// etcdRollback rollsback up a failed upgrade attempt to the previous state
func etcdRollback() error {
	return nil
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
