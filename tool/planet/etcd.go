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

package main

import (
	"bufio"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/gravitational/planet/lib/box"
	"github.com/gravitational/planet/lib/constants"
	"github.com/gravitational/planet/lib/monitoring"
	"github.com/gravitational/planet/lib/utils"

	"github.com/coreos/go-systemd/dbus"
	"github.com/davecgh/go-spew/spew"
	etcdconf "github.com/gravitational/coordinate/config"
	backup "github.com/gravitational/etcd-backup/lib/etcd"
	"github.com/gravitational/trace"
	ps "github.com/mitchellh/go-ps"
	log "github.com/sirupsen/logrus"
	etcd "go.etcd.io/etcd/client"
	etcdv3 "go.etcd.io/etcd/clientv3"
)

// etcdInit detects which version of etcd should be running, and sets symlinks to point
// to the correct version
func etcdInit() error {
	desiredVersion, _, err := readEtcdVersion(DefaultPlanetReleaseFile)
	if err != nil {
		return trace.Wrap(err)
	}
	log.Info("Desired etcd version: ", desiredVersion)

	currentVersion, _, err := readEtcdVersion(DefaultEtcdCurrentVersionFile)
	if err != nil {
		if !trace.IsNotFound(err) {
			return trace.Wrap(err)
		}
		currentVersion = AssumeEtcdVersion

		// if the etcd data directory doesn't exist, treat this as a new installation
		if _, err := os.Stat("/ext/etcd/member"); os.IsNotExist(err) {
			// If the etcd data directory doesn't exist, we can assume this
			// is a new install of etcd, and use the latest version.
			log.Info("New installation detected, using etcd version: ", desiredVersion)
			err = writeEtcdEnvironment(DefaultEtcdCurrentVersionFile, desiredVersion, "")
			if err != nil {
				return trace.Wrap(err)
			}
			currentVersion = desiredVersion
		}
	}
	log.Info("Current etcd version: ", currentVersion)

	// symlink /usr/bin/etcd to the version we expect to be running
	// Note: we wrap etcdctl in a shell script to wipe any proxy env variables when running. So the path to the etcdctl
	// binary is actually etcdctl-cmd
	for _, path := range []string{"/usr/bin/etcd", "/usr/bin/etcdctl-cmd"} {
		// ignore the error from os.Remove, since we don't care if it fails
		_ = os.Remove(path)
		err = os.Symlink(
			fmt.Sprint(path, "-", currentVersion),
			path,
		)
		if err != nil {
			return trace.ConvertSystemError(err)
		}
	}

	// create a symlink for the etcd data
	// this way we can easily support upgrade/rollback by simply changing
	// the pointer to where the data lives
	// Note: in order to support rollback to version 2.3.8, we need to link
	// to /ext/data
	dest := getBaseEtcdDir(currentVersion)
	err = os.MkdirAll(dest, 0700)
	if err != nil && !os.IsExist(err) {
		return trace.ConvertSystemError(err)
	}

	// chown the destination directory to the planet user
	fi, err := os.Stat(DefaultEtcdStoreBase)
	if err != nil {
		return trace.ConvertSystemError(err)
	}
	stat := fi.Sys().(*syscall.Stat_t)
	uid := int(stat.Uid)
	gid := int(stat.Gid)
	err = chownDir(dest, uid, gid)
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}

func etcdBackup(backupFile string, backupPrefix []string) (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), EtcdUpgradeTimeout)
	defer cancel()

	writer := os.Stdout
	if backupFile != "" {
		writer, err = os.Create(backupFile)
		if err != nil {
			return trace.Wrap(err)
		}
		defer writer.Close()
	}

	backupConf := backup.BackupConfig{
		EtcdConfig: etcdconf.Config{
			Endpoints: []string{DefaultEtcdEndpoints},
			KeyFile:   DefaultEtcdctlKeyFile,
			CertFile:  DefaultEtcdctlCertFile,
			CAFile:    DefaultEtcdctlCAFile,
		},
		Prefix: backupPrefix,
		Writer: writer,
		Log:    log.StandardLogger(),
	}
	log.Info("BackupConfig: ", spew.Sdump(backupConf))

	err = backup.Backup(ctx, backupConf)
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// etcdDisable disables etcd on this machine
// Used during upgrades
func etcdDisable(upgradeService, stopAPIServer bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), EtcdUpgradeTimeout)
	defer cancel()

	// Kevin: Workaround, for the API server presenting stale data to clients while etcd is down. Make sure we shut down
	// the API server as well (passed as flag from gravity to prevent accidental usage).
	// TODO: This fix needs to be revisited to include a permanent solution.
	if stopAPIServer {
		err := systemctl(ctx, "stop", APIServerServiceName)
		if err != nil {
			return trace.Wrap(err)
		}
	}

	if upgradeService {
		return trace.Wrap(disableService(ctx, ETCDUpgradeServiceName))
	}

	return trace.Wrap(disableService(ctx, ETCDServiceName))
}

// etcdEnable enables a disabled etcd node
func etcdEnable(upgradeService bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), EtcdUpgradeTimeout)
	defer cancel()

	if !upgradeService {
		// restart the clients of the etcd service when the etcd service is brought online, which usually will be post
		// upgrade. This will ensure clients running inside planet are restarted, which will refresh any local state
		restartEtcdClients(ctx)
		return trace.Wrap(enableService(ctx, ETCDServiceName))
	}
	// don't actually enable the service if this is a proxy
	env, err := box.ReadEnvironment(ContainerEnvironmentFile)
	if err != nil {
		return trace.Wrap(err)
	}

	if env.Get(EnvEtcdProxy) == EtcdProxyOn {
		log.Info("etcd is in proxy mode, nothing to do")
		return nil
	}
	return trace.Wrap(enableService(ctx, ETCDUpgradeServiceName))
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
		log.Info("etcd is in proxy mode, nothing to do")
		return nil
	}

	log.Info("Checking etcd service status")
	services := []string{ETCDServiceName, ETCDUpgradeServiceName}
	for _, service := range services {
		status, err := getServiceStatus(service)
		if err != nil {
			log.Warnf("Failed to query status of service %v. Continuing upgrade. Error: %v", service, err)
			continue
		}
		log.Infof("%v service status: %v", service, status)
		if status != "inactive" && status != "failed" {
			return trace.BadParameter("%v must be disabled in order to run the upgrade. current status: %v", service, status)
		}
	}

	// In order to upgrade in a re-entrant way
	// we need to make sure that if the upgrade or rollback is repeated
	// that it skips anything that has been done on a previous run, and continues anything that may have failed
	desiredVersion, _, err := readEtcdVersion(DefaultPlanetReleaseFile)
	if err != nil {
		return trace.Wrap(err)
	}
	log.Info("Desired etcd version: ", desiredVersion)

	currentVersion, backupVersion, err := readEtcdVersion(DefaultEtcdCurrentVersionFile)
	if err != nil {
		if trace.IsNotFound(err) {
			currentVersion = AssumeEtcdVersion
		} else {
			return trace.Wrap(err)
		}
	}
	log.Info("Current etcd version: ", currentVersion)
	log.Info("Backup etcd version: ", backupVersion)

	if rollback {
		// in order to rollback, write the backup version as the current version, with no backup version
		if backupVersion != "" {
			err = writeEtcdEnvironment(DefaultEtcdCurrentVersionFile, backupVersion, "")
			if err != nil {
				return trace.Wrap(err)
			}
		}
	} else {
		// in order to upgrade, write the new version to disk with the current version as backup
		// if current version == desired version, we must have already run this step
		if currentVersion != desiredVersion {
			err = writeEtcdEnvironment(DefaultEtcdCurrentVersionFile, desiredVersion, currentVersion)
			if err != nil {
				return trace.Wrap(err)
			}

			// wipe old backups leftover from previous upgrades
			// Note: if this fails, but previous steps were successfull, the backups won't get cleaned up
			if backupVersion != "" {
				path := path.Join(getBaseEtcdDir(backupVersion), "member")
				err = os.RemoveAll(path)
				if err != nil {
					return trace.ConvertSystemError(err)
				}
			}
		}

		// wipe data directory of any previous upgrade attempt
		path := path.Join(getBaseEtcdDir(desiredVersion), "member")
		err = os.RemoveAll(path)
		if err != nil {
			return trace.ConvertSystemError(err)
		}
	}

	log.Info("Upgrade complete")

	return nil
}

// restartEtcdClients - because the etcd cluster has been recreated, all clients need to be refreshed so their
// watches are not pointing at incorrect revisions.
func restartEtcdClients(ctx context.Context) {
	services := []string{APIServerServiceName, PlanetAgentServiceName, FlannelServiceName, ProxyServiceName,
		KubeletServiceName, CorednsServiceName}

	for _, service := range services {
		// reset the kubernetes api server to take advantage of any new etcd settings that may have changed
		// this only happens if the service is already running
		status, err := getServiceStatus(service)
		if err != nil {
			log.WithFields(log.Fields{
				log.ErrorKey: err,
				"service":    service,
			}).Warn("Failed to query service status.")
			return
		}
		if status != "inactive" {
			tryResetService(ctx, service)
		}
	}
}

// startWatchingEtcdMasters creates a control loop which polls etcd for the etcd cluster member list, and updates the
// etcd gateway configuration with any changes. This keeps the etcd gateway load balancing in sync with the cluster.
func startWatchingEtcdMasters(ctx context.Context, config *monitoring.Config) error {
	cli, err := config.ETCDConfig.NewClientV3()
	if err != nil {
		return trace.Wrap(err)
	}

	go watchEtcdMasters(ctx, cli)
	return nil
}

func watchEtcdMasters(ctx context.Context, client *etcdv3.Client) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	endpoints := strings.Split(os.Getenv(EnvEtcdGatewayEndpoints), ",")
	sort.Strings(endpoints)
	gateway := etcdGateway{
		clientURLs: endpoints,
	}

	for {
		select {
		case <-ticker.C:
			err := gateway.resyncEtcdMasters(ctx, client)
			if err != nil {
				log.WithError(err).Warn("Error resyncing etcd master list.")
			}
		case <-ctx.Done():
			return
		}
	}
}

type etcdGateway struct {
	clientURLs []string
}

func (e *etcdGateway) resyncEtcdMasters(ctx context.Context, client *etcdv3.Client) error {
	memberList, err := client.MemberList(ctx)
	if err != nil {
		return trace.Wrap(err, "error retrieving member list")
	}

	newClientURLs, err := collectClientURLs(memberList)
	if err != nil {
		return trace.Wrap(err)
	}

	// Only rewrite the configuration if there are changes
	sort.Strings(newClientURLs)
	if reflect.DeepEqual(newClientURLs, e.clientURLs) {
		return nil
	}

	env := fmt.Sprintf("%v=%q", EnvEtcdGatewayEndpoints, strings.Join(newClientURLs, ","))
	log.WithField("file", DefaultEtcdSyncedEnvFile).Info("Updating etcd gateway environment: ", env)
	err = utils.SafeWriteFile(DefaultEtcdSyncedEnvFile, []byte(env), constants.SharedReadMask)
	if err != nil {
		return trace.Wrap(err, "failed to update etcd environment file").AddField("file", DefaultEtcdSyncedEnvFile)
	}

	err = systemctl(ctx, "restart", ETCDServiceName)
	if err != nil {
		return trace.Wrap(err, "failed to restart etcd service").AddField("service", ETCDServiceName)
	}

	e.clientURLs = newClientURLs
	return nil
}

func collectClientURLs(memberList *etcdv3.MemberListResponse) ([]string, error) {
	newClientURLs := []string{}
	for _, member := range memberList.Members {
		memberURLs := member.GetClientURLs()
		if len(memberURLs) == 0 {
			return nil, trace.BadParameter("etcd member doesn't have any client urls")
		}

		// Only use the first memberUrl to prevent the same member appearing multiple times
		u, err := url.Parse(memberURLs[0])
		if err != nil {
			return nil, trace.Wrap(err, "error parsing etcd member url").AddField("url", memberURLs[0])
		}

		newClientURLs = append(newClientURLs, u.Host)
	}
	return newClientURLs, nil
}

func getBaseEtcdDir(version string) string {
	p := DefaultEtcdStoreBase
	if version != AssumeEtcdVersion {
		p = filepath.Join(DefaultEtcdStoreBase, version)
	}
	return p
}

func etcdRestore(file string) error {
	log.Info("Initializing new etcd database")
	err := etcdEnable(true)
	if err != nil {
		return trace.Wrap(err)
	}

	etcdConf := etcdconf.Config{
		Endpoints: []string{DefaultEtcdUpgradeEndpoints},
		KeyFile:   DefaultEtcdctlKeyFile,
		CertFile:  DefaultEtcdctlCertFile,
		CAFile:    DefaultEtcdctlCAFile,
	}
	client, err := etcdConf.NewClient()
	if err != nil {
		return trace.Wrap(err)
	}

	// wait for the temporary etcd instance to complete startup
	log.Info("Waiting for etcd initialization to complete")
	err = waitEtcdHealthyTimeout(waitCtx, 1*time.Minute, client)
	if err != nil {
		return trace.Wrap(err)
	}

	// stop etcd now that it's DB is initialized but empty, to run offline backups
	log.Info("Etcd initialization complete, stopping")
	err = etcdDisable(true, false)
	if err != nil {
		return trace.Wrap(err)
	}

	// run offline restoration steps
	restoreConf := backup.RestoreConfig{
		Prefix: []string{""}, // Restore all etcd data
		File:   file,
	}
	log.Info("Offline RestoreConfig: ", spew.Sdump(restoreConf))
	restoreConf.Log = log.StandardLogger()

	datadir, err := getEtcdDataDir()
	if err != nil {
		return trace.Wrap(err)
	}

	log.Info("Starting offline etcd restoration")
	err = backup.OfflineRestore(context.TODO(), restoreConf, datadir)
	if err != nil {
		return trace.Wrap(err)
	}

	// start etcd for running online restoration steps
	log.Info("Starting temporary etcd cluster for online restoration")
	err = etcdEnable(true)
	if err != nil {
		return trace.Wrap(err)
	}

	log.Info("Waiting for etcd ")
	err = waitEtcdHealthyTimeout(waitCtx, 1*time.Minute, client)
	if err != nil {
		return trace.Wrap(err)
	}

	log.Info("Restoring backup to temporary etcd")
	restoreConf = backup.RestoreConfig{
		EtcdConfig:    etcdConf,
		Prefix:        []string{"/"},                // Restore all etcd data
		MigratePrefix: []string{ETCDRegistryPrefix}, // migrate kubernetes data to etcd3 datastore
		File:          file,
		SkipV3:        true,
	}
	log.Info("Online RestoreConfig: ", spew.Sdump(restoreConf))
	restoreConf.Log = log.StandardLogger()

	log.Info("Starting online restoration")
	err = backup.Restore(context.TODO(), restoreConf)
	if err != nil {
		return trace.Wrap(err)
	}

	// stop etcd now that the restoration is completed, gravity will coordinate the restart of the cluster
	log.Info("Stopping temporary etcd cluster")
	err = etcdDisable(true, false)
	if err != nil {
		return trace.Wrap(err)
	}

	log.Info("Restore complete")
	return nil
}

func waitEtcdHealthyTimeout(ctx context.Context, timeout time.Duration, client etcd.Client) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return trace.Wrap(waitEtcdHealthy(ctx, client))
}

// waitEtcdHealthy waits for etcd to have a leader elected
func waitEtcdHealthy(ctx context.Context, client etcd.Client) error {
	mapi := etcd.NewMembersAPI(client)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return trace.Wrap(ctx.Err())
		case <-ticker.C:
			leader, _ := mapi.Leader(ctx)
			if leader != nil {
				return nil
			}
		}
	}
}

// etcdWipe wipes out all local etcd data
func etcdWipe(confirmed bool) error {
	dataDir, err := getEtcdDataDir()
	if err != nil {
		return trace.Wrap(err)
	}
	if !confirmed {
		err := getConfirmation(fmt.Sprintf(wipeoutPrompt, dataDir),
			wipeoutConfirmation)
		if err != nil {
			return trace.Wrap(err)
		}
	}
	log.Warnf("Deleting etcd data at %v.", dataDir)
	err = os.RemoveAll(dataDir)
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// wipeoutConfirmation is the expected user response to confirm data delete action
const wipeoutConfirmation = "yes"

// wipeoutPrompt is the user prompt for data delete action
var wipeoutPrompt = "Danger! This operation will delete all etcd data in %v " +
	"and is not reversible. Type '" + wipeoutConfirmation + "' to proceed."

// getEtcdDataDir returns full path to etcd data directory
func getEtcdDataDir() (string, error) {
	version, _, err := readEtcdVersion(DefaultEtcdCurrentVersionFile)
	if err != nil && !trace.IsNotFound(err) {
		return "", trace.Wrap(err)
	}
	if trace.IsNotFound(err) {
		version = AssumeEtcdVersion
	}
	return getBaseEtcdDir(version), nil
}

// getConfirmation obtains action confirmation from the user
func getConfirmation(prompt, confirmationResponse string) error {
	fmt.Printf("%v ", prompt)
	userResponse, err := bufio.NewReader(os.Stdin).ReadSlice('\n')
	if err != nil {
		return trace.Wrap(err)
	}
	if strings.TrimSpace(string(userResponse)) != confirmationResponse {
		return trace.BadParameter("action cancelled by user")
	}
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
func systemctl(ctx context.Context, operation, service string) error {
	out, err := exec.CommandContext(ctx, "/bin/systemctl", "--no-block", operation, service).CombinedOutput()
	log.Infof("%v %v: %v", operation, service, string(out))
	if err != nil {
		return trace.Wrap(err, "failed to %v %v: %v", operation, service, string(out))
	}
	return nil
}

// waitForEtcdStopped waits for etcd to not be present in the process list
// the problem is, when shutting down etcd, systemd will respond when the process has been told to shutdown
// but this leaves a race, where we might be continuing while etcd is still cleanly shutting down
func waitForEtcdStopped(ctx context.Context) error {
	ticker := time.NewTicker(WaitInterval)
	defer ticker.Stop()
loop:
	for {
		select {
		case <-ctx.Done():
			return trace.Wrap(ctx.Err())
		case <-ticker.C:
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
func tryResetService(ctx context.Context, service string) {
	// ignoring error results is intentional
	err := systemctl(ctx, "restart", service)
	if err != nil {
		log.Warn("error attempting to restart service", err)
	}
}

func disableService(ctx context.Context, service string) error {
	err := systemctl(ctx, "mask", service)
	if err != nil {
		return trace.Wrap(err)
	}
	err = systemctl(ctx, "stop", service)
	if err != nil {
		return trace.Wrap(err)
	}
	return trace.Wrap(waitForEtcdStopped(ctx))
}

func enableService(ctx context.Context, service string) error {
	err := systemctl(ctx, "unmask", service)
	if err != nil {
		return trace.Wrap(err)
	}
	return trace.Wrap(systemctl(ctx, "start", service))
}

func getServiceStatus(service string) (string, error) {
	conn, err := dbus.New()
	if err != nil {
		return "", trace.Wrap(err)
	}
	defer conn.Close()

	status, err := conn.ListUnitsByNames([]string{service})
	if err != nil {
		return "", trace.Wrap(err)
	}
	if len(status) != 1 {
		return "", trace.BadParameter("unexpected number of status results when checking service '%q'", service)
	}

	return status[0].ActiveState, nil
}

func readEtcdVersion(path string) (currentVersion string, prevVersion string, err error) {
	inFile, err := os.Open(path)
	if err != nil {
		return "", "", trace.ConvertSystemError(err)
	}
	defer inFile.Close()

	scanner := bufio.NewScanner(inFile)
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		line := scanner.Text()

		if strings.Contains(line, "=") {
			split := strings.SplitN(line, "=", 2)
			if len(split) == 2 {
				switch split[0] {
				case EnvEtcdVersion:
					currentVersion = split[1]
				case EnvEtcdPrevVersion:
					prevVersion = split[1]
				}
			}
		}
	}

	if currentVersion == "" {
		return "", "", trace.NotFound("unable to parse etcd version")
	}
	return currentVersion, prevVersion, nil
}

func writeEtcdEnvironment(path string, version string, prevVersion string) error {
	err := os.MkdirAll(filepath.Dir(path), constants.SharedReadMask)
	if err != nil {
		return trace.ConvertSystemError(err)
	}

	f, err := os.Create(path)
	if err != nil {
		return trace.ConvertSystemError(err)
	}
	defer f.Close()

	// don't write the version during rollback to 2.3.8 because the systemd unit file
	// use the version to locate the data directory. When rolling back, we want to rollback to
	// a nil directory
	if version != AssumeEtcdVersion {
		_, err = fmt.Fprint(f, EnvEtcdVersion, "=", version, "\n")
		if err != nil {
			return err
		}
	}

	if prevVersion != "" {
		_, err = fmt.Fprint(f, EnvEtcdPrevVersion, "=", prevVersion, "\n")
		if err != nil {
			return err
		}
	}

	backend := "etcd3"
	if version == AssumeEtcdVersion {
		backend = "etcd2"
	}
	_, err = fmt.Fprint(f, EnvStorageBackend, "=", backend, "\n")
	if err != nil {
		return err
	}

	return nil
}
