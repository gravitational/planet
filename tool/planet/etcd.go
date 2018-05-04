package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	etcd "github.com/coreos/etcd/client"
	"github.com/coreos/go-systemd/dbus"
	"github.com/davecgh/go-spew/spew"
	etcdconf "github.com/gravitational/coordinate/config"
	backup "github.com/gravitational/etcd-backup/lib/etcd"
	"github.com/gravitational/planet/lib/box"
	"github.com/gravitational/trace"
	ps "github.com/mitchellh/go-ps"
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
	if err := os.RemoveAll(ETCDProxyDir); err != nil && !os.IsNotExist(err) {
		return trace.Wrap(err)
	}

	setupEtcd(&Config{
		Rootfs:    "/",
		EtcdProxy: "off",
	})

	out, err = exec.Command("/bin/systemctl", "daemon-reload").CombinedOutput()
	log.Infof("systemctl daemon-reload: %v", string(out))
	if err != nil {
		return trace.Wrap(err, fmt.Sprintf("failed to trigger systemctl daemon-reload: %v", string(out)))
	}

	out, err = exec.Command("/bin/systemctl", "start", ETCDServiceName).CombinedOutput()
	log.Infof("starting etcd: %v", string(out))
	if err != nil {
		return trace.Wrap(err, fmt.Sprintf("failed to start etcd: %v", string(out)))
	}

	out, err = exec.Command("/bin/systemctl", "restart", PlanetAgentServiceName).CombinedOutput()
	log.Infof("restarting planet-agent: %v", string(out))
	if err != nil {
		return trace.Wrap(err, fmt.Sprintf("failed to restart planet-agent: %v", string(out)))
	}

	return nil
}

// etcdInit detects which version of etcd should be running, and sets symlinks to point
// to the correct version
func etcdInit() error {
	currentVersion, err := currentEtcdVersion(DefaultEtcdCurrentVersionFile)
	if err != nil {
		return trace.Wrap(err)
	}
	log.Info("Current etcd version: ", currentVersion)

	desiredVersion, err := readEtcdVersion(DefaultEtcdDesiredVersionFile)
	if err != nil {
		return trace.Wrap(err)
	}
	log.Info("Desired etcd version: ", desiredVersion)

	if currentVersion == AssumeEtcdVersion {
		if _, err := os.Stat("/ext/etcd/member"); os.IsNotExist(err) {
			// If the etcd data directory doesn't exist, we can assume this
			// is a new install of etcd, and use the latest version.
			log.Info("new installation detected, using etcd version: ", desiredVersion)
			err = writeEtcdEnvironment(DefaultEtcdCurrentVersionFile, desiredVersion)
			if err != nil {
				return trace.Wrap(err)
			}
			currentVersion = desiredVersion
		}
	}

	// symlink /usr/bin/etcd to the version we expect to be running
	for _, path := range []string{"/usr/bin/etcd", "/usr/bin/etcdctl"} {
		// intentioned ignore the error from os.Remove, since we don't care if it fails
		_ = os.Remove(path)
		err = os.Symlink(
			fmt.Sprint(path, "-", currentVersion),
			path,
		)
		if err != nil {
			return trace.Wrap(err)
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

// etcdDisable disabled etcd on this machine
// Used during upgrades
func etcdDisable(upgradeService bool) error {
	if upgradeService {
		return trace.Wrap(disableService(ETCDUpgradeServiceName))
	}
	return trace.Wrap(disableService(ETCDServiceName))
}

// etcdEnable enables a disabled etcd node
func etcdEnable(upgradeService bool) error {
	if upgradeService {
		// don't actually enable the service if this is a proxy
		env, err := box.ReadEnvironment(ContainerEnvironmentFile)
		if err != nil {
			return trace.Wrap(err)
		}

		if env.Get(EnvEtcdProxy) == EtcdProxyOn {
			log.Infof("etcd is in proxy mode, nothing to do")
			return nil
		}
		return trace.Wrap(enableService(ETCDUpgradeServiceName))
	}
	return trace.Wrap(enableService(ETCDServiceName))

}

// etcdUpgrade upgrades / rollbacks the etcd upgrade
// the procedure is basically the same for an upgrade or rollback, just with some paths reversed
func etcdUpgrade(rollback bool) error {
	log.Info("Updating etcd")

	env, err := box.ReadEnvironment(ContainerEnvironmentFile)
	if err != nil {
		return trace.Wrap(err)
	}

	if env.Get(EnvEtcdProxy) == EtcdProxyOn {
		log.Infof("etcd is in proxy mode, nothing to do")
		return nil
	}

	log.Info("Checking etcd service status")
	services := []string{ETCDServiceName, ETCDUpgradeServiceName}
	for _, service := range services {
		status, err := getServiceStatus(service)
		if err != nil {
			return trace.Wrap(err)
		}
		log.Info("%v service status: %v", service, status)
		if status != "inactive" {
			return trace.BadParameter("%v must be disabled in order to run the upgrade", service)
		}
	}

	versionFile := DefaultEtcdCurrentVersionFile
	if rollback {
		versionFile = DefaultEtcdBackupVersionFile
	}
	currentVersion, err := currentEtcdVersion(versionFile)
	if err != nil {
		return trace.Wrap(err)
	}
	log.Info("Current etcd version: ", currentVersion)

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

	// Upgrade - Move the current data directory to the backup location
	// Rollback - Move the backup back to the original directory
	// removes the destination directory before moving
	log.Info("Backup etcd data")
	from := DefaultEtcdStoreCurrent
	to := DefaultEtcdStoreBackup
	if rollback {
		from = DefaultEtcdStoreBackup
		to = DefaultEtcdStoreCurrent
	}
	err = os.RemoveAll(to)
	if err != nil {
		return trace.Wrap(err)
	}
	err = os.Rename(from, to)
	if err != nil {
		return trace.Wrap(err)
	}

	// Upgrade - Move the current version file to the backup location
	// Rollback - Move the backup version to the original directory
	log.Info("Backup etcd data")
	from = DefaultEtcdCurrentVersionFile
	to = DefaultEtcdBackupVersionFile
	if rollback {
		from = DefaultEtcdBackupVersionFile
		to = DefaultEtcdCurrentVersionFile
	}
	err = os.RemoveAll(to)
	if err != nil && !os.IsNotExist(err) {
		return trace.Wrap(err)
	}
	err = os.Rename(from, to)
	if err != nil && !os.IsNotExist(err) {
		return trace.Wrap(err)
	}

	//only write the new version information if doing an upgrade
	if !rollback {
		// Write desired version as the current version file
		log.Info("Writing etcd desired version: ", desiredVersion)
		err = writeEtcdEnvironment(DefaultEtcdCurrentVersionFile, desiredVersion)
		if err != nil {
			return trace.Wrap(err)
		}
	}

	// reset the kubernetes api server to take advantage of any new etcd settings that may have changed
	// this only happens if the service is already running
	status, err := getServiceStatus(APIServerServiceName)
	if err != nil {
		return trace.Wrap(err)
	}
	if status != "inactive" {
		tryResetService(APIServerServiceName)
	}

	log.Info("Upgrade complete")

	return nil
}

func etcdRestore(file string) error {
	ctx := context.TODO()
	log.Info("Restoring backup to temporary etcd")
	restoreConf := backup.RestoreConfig{
		EtcdConfig: etcdconf.Config{
			Endpoints: []string{DefaultEtcdUpgradeEndpoints},
			KeyFile:   DefaultEtcdctlKeyFile,
			CertFile:  DefaultEtcdctlCertFile,
			CAFile:    DefaultEtcdctlCAFile,
		},
		Prefix:        []string{"/"},         // Restore all etcd data
		MigratePrefix: []string{"/registry"}, // migrate kubernetes data to etcd3 datastore
		File:          file,
	}
	log.Info("RestoreConfig: ", spew.Sdump(restoreConf))
	restoreConf.Log = log.StandardLogger()

	err := backup.Restore(ctx, restoreConf)
	if err != nil {
		return trace.Wrap(err)
	}

	log.Info("Restore complete")
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

// systemctl runs a local systemctl command.
// TODO(knisbet): I'm using systemctl here, because using go-systemd and dbus appears to be unreliable, with
// masking unit files not working. Ideally, this will use dbus at some point in the future.
func systemctl(operation, service string) error {
	ctx, _ := context.WithTimeout(context.Background(), 1*time.Minute)
	out, err := exec.CommandContext(ctx, "/bin/systemctl", "--no-block", operation, service).CombinedOutput()
	log.Infof("%v %v: %v", operation, service, string(out))
	if err != nil {
		return trace.Wrap(err, fmt.Sprintf("failed to %v %v: %v", operation, service, string(out)))
	}
	return nil
}

// waitForEtcdStopped waits for etcd to not be present in the process list
// the problem is, when shutting down etcd, systemd will respond when the process has been told to shutdown
// but this leaves a race, where we might be continuing while etcd is still cleanly shutting down
func waitForEtcdStopped() error {
	ctx, _ := context.WithTimeout(context.Background(), 1*time.Minute)
loop:
	for {
		select {
		case <-ctx.Done():
			return trace.Wrap(ctx.Err())
		default:
		}

		procs, err := ps.Processes()
		if err != nil {
			return trace.Wrap(err)
		}
		for _, proc := range procs {
			if proc.Executable() == "etcd" {
				continue loop
			}
		}
		return nil
	}
}

// tryResetService will request for systemd to restart a system service
func tryResetService(service string) {
	// ignoring error results is intentional
	_ = systemctl("restart", service)
}

func disableService(service string) error {
	err := systemctl("mask", service)
	if err != nil {
		return trace.Wrap(err)
	}
	err = systemctl("stop", service)
	if err != nil {
		return trace.Wrap(err)
	}
	return trace.Wrap(waitForEtcdStopped())
}

func enableService(service string) error {
	err := systemctl("unmask", service)
	if err != nil {
		return trace.Wrap(err)
	}
	return trace.Wrap(systemctl("start", service))
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
	err := os.MkdirAll(filepath.Dir(path), 644)
	if err != nil {
		return trace.Wrap(err)
	}

	f, err := os.Create(path)
	if err != nil {
		return trace.Wrap(err)
	}
	defer f.Close()

	_, err = f.WriteString(fmt.Sprint(EnvEtcdVersion, "=", version, "\n"))
	if err != nil {
		return err
	}

	_, err = f.WriteString(fmt.Sprint(EnvStorageBackend, "=", "etcd3", "\n"))
	if err != nil {
		return err
	}

	return nil
}
