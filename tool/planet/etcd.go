package main

import (
	"bufio"
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
	}
	log.Info("BackupConfig: ", spew.Sdump(backupConf))
	backupConf.Log = log.StandardLogger()

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
	return trace.Wrap(disableService(ETCDServiceName))
}

// etcdEnable enables a disabled etcd node
func etcdEnable() error {
	return trace.Wrap(enableService(ETCDServiceName))

}

// etcdUpgradeMaster upgrades the first server in the cluster
func etcdUpgradeMaster(file string) error {
	ctx := context.TODO()
	log.Info("Updating first server in the cluster")

	err := etcdUpgradeCommon()
	if err != nil {
		return trace.Wrap(err)
	}

	log.Info("Launching temporary etcd instance")
	err = enableService(ETCDUpgradeServiceName)
	if err != nil {
		return trace.Wrap(err)
	}

	log.Info("Restoring backup to temporary etcd")
	restoreConf := backup.RestoreConfig{
		EtcdConfig: etcdconf.Config{
			Endpoints: []string{DefaultEtcdUpgradeEndpoints},
		},
		Prefix:        []string{"/"},         // Restore all etcd data
		MigratePrefix: []string{"/registry"}, // migrate kubernetes data to etcd3 datastore
		File:          file,
	}
	log.Info("RestoreConfig: ", spew.Sdump(restoreConf))
	restoreConf.Log = log.StandardLogger()

	err = backup.Restore(ctx, restoreConf)
	if err != nil {
		return trace.Wrap(err)
	}

	log.Info("Disabling temporary etcd instance")
	err = disableService(ETCDUpgradeServiceName)
	if err != nil {
		return trace.Wrap(err)
	}

	log.Info("Upgrade complete")

	return nil
}

// etcdUpgradeSlave upgrades additional masters or proxies in etcd
func etcdUpgradeSlave() error {
	return nil
}

func etcdUpgradeCommon() error {
	log.Info("Starting common etcd upgrade steps")

	log.Info("Checking etcd service status")
	status, err := getServiceStatus(ETCDServiceName)
	if err != nil {
		return trace.Wrap(err)
	}
	log.Info("Etcd service status: ", status)
	if status != "inactive" {
		return trace.BadParameter("Etcd must be disabled in order to run the upgrade")
	}

	log.Info("Find current etcd version")
	currentVersion, err := currentEtcdVersion(DefaultEtcdCurrentVersionFile)
	if err != nil {
		return trace.Wrap(err)
	}
	log.Info("Current etcd version: ", currentVersion)

	log.Info("Find desired etcd version")
	desiredVersion, err := readEtcdVersion(DefaultEtcdDesiredVersionFile)
	if err != nil {
		return trace.Wrap(err)
	}
	log.Info("Desired etcd version: ", desiredVersion)

	// we're already on the correct version
	if currentVersion == desiredVersion {
		log.Info("Already running desired versions")
		return nil
	}

	// Remove previous backup of etcd data directory if it already exists
	log.Info("Cleaning up data from previous upgrades")
	err = os.RemoveAll(DefaultEtcdStoreBackup)
	if err != nil {
		return trace.Wrap(err)
	}

	// Move the current data directory to the backup location
	log.Info("Backup etcd data")
	err = os.Rename(DefaultEtcdStoreCurrent, DefaultEtcdStoreBackup)
	if err != nil {
		return trace.Wrap(err)
	}

	// Write desired version as the current version file
	log.Info("Writign etcd desired version: ", desiredVersion)
	err = writeEtcdEnvironment(DefaultEtcdCurrentVersionFile, desiredVersion)
	if err != nil {
		return trace.Wrap(err)
	}

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

func disableService(service string) error {
	conn, err := dbus.New()
	if err != nil {
		return trace.Wrap(err)
	}

	changes, err := conn.MaskUnitFiles([]string{service}, false, true)
	if err != nil {
		return trace.Wrap(err)
	}
	for _, change := range changes {
		log.Debugf("unmask: type: %v filename: %v destination: %v", change.Type, change.Filename, change.Destination)
	}

	c := make(chan string)
	_, err = conn.StopUnit(service, "replace", c)
	if err != nil {
		return trace.Wrap(err)
	}

	status := <-c
	if strings.ToLower(status) != "done" {
		return trace.BadParameter("Systemd stop unit recieved unexpected result: %v", status)
	}
	return nil
}

func enableService(service string) error {
	conn, err := dbus.New()
	if err != nil {
		return trace.Wrap(err)
	}

	changes, err := conn.UnmaskUnitFiles([]string{service}, false)
	if err != nil {
		return trace.Wrap(err)
	}
	for _, change := range changes {
		log.Debugf("unmask: type: %v filename: %v destination: %v", change.Type, change.Filename, change.Destination)
	}

	c := make(chan string)
	//https://godoc.org/github.com/coreos/go-systemd/dbus#Conn.StartUnit
	_, err = conn.StartUnit(service, "replace", c)
	if err != nil {
		return trace.Wrap(err)
	}

	status := <-c
	if strings.ToLower(status) != "done" {
		return trace.BadParameter("Systemd start unit recieved unexpected result: %v", status)
	}
	return nil
}

func getServiceStatus(service string) (string, error) {
	conn, err := dbus.New()
	if err != nil {
		return "", trace.Wrap(err)
	}

	status, err := conn.ListUnitsByNames([]string{service})
	if err != nil {
		return "", trace.Wrap(err)
	}
	if len(status) != 1 {
		return "", trace.BadParameter("Unexpected number of status results when checking service '%v'", service)
	}

	return status[0].ActiveState, nil
}

// currentEtcdVersion tries to read the version, but if the file doesn't exist returns an assumed version
func currentEtcdVersion(path string) (string, error) {
	if _, err := os.Stat(path); err == nil {
		return readEtcdVersion(path)
	}
	return AssumeEtcdVersion, nil
}

func readEtcdVersion(path string) (string, error) {
	inFile, err := os.Open(path)
	if err != nil {
		return "", trace.Wrap(err)
	}
	defer inFile.Close()

	scanner := bufio.NewScanner(inFile)
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		line := scanner.Text()

		if strings.Contains(line, "=") {
			split := strings.SplitN(line, "=", 2)
			if len(split) == 2 {
				if strings.ToUpper(split[0]) == EnvEtcdVersion {
					return split[1], nil
				}
			}
		}
	}

	return "", trace.BadParameter("unable to parse etcd version")
}

func writeEtcdEnvironment(path string, version string) error {
	f, err := os.Create(path)
	if err != nil {
		return trace.Wrap(err)
	}
	defer f.Close()

	_, err = f.WriteString(fmt.Sprint(EnvEtcdVersion, "=", version))
	if err != nil {
		return err
	}

	return nil
}
