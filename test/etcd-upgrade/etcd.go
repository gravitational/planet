/*
Copyright 2020 Gravitational, Inc.

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

package etcd

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/gravitational/planet/test/internal/etcd"

	etcdconf "github.com/gravitational/coordinate/config"
	backup "github.com/gravitational/etcd-backup/lib/etcd"
	"github.com/gravitational/trace"
	"github.com/sirupsen/logrus"
	clientv2 "go.etcd.io/etcd/client"
	"go.etcd.io/etcd/clientv3"
)

const etcdTestContainerName = "planet-etcd-upgrade-test-0"
const etcdImage = "gcr.io/etcd-development/etcd"
const etcdPort = "22379"
const etcdUpgradePort = "22380"

// TestUpgradeBetweenVersions runs a mock-up of the etcd upgrade process used by planet and triggered by gravity on
// a multi-node cluster. The overall upgrade process is described by gravity here:
// https://github.com/gravitational/gravity/blob/ddcf66dbb599138cc79fc1b9be427d706c6c8fb8/lib/update/cluster/phases/etcd.go
//
// The specific items this test is meant to validate are:
// - Watches - watches rely on the revision for tracking changes, so the upgrade process needs to preserve the revision
// - Internals - To maintain the revision, we have to internally change the etcd database, so test for stability
//
// Note - Only watches against etcd v3 are being fixed, etcd v2 clients need to be restarted when doing an upgrade
// as in the future, all etcd clients will be upgraded to v3, or we'll use the v2 API emulation with the v3 datastore
// which as of Jan 2020 is experimental.
func TestUpgradeBetweenVersions(from, to string) error {
	etcdDir, err := ioutil.TempDir("", "etcd")
	assertNoErr(err)
	defer os.RemoveAll(etcdDir)
	logrus.WithField("dir", etcdDir).Info("temp directory")

	dataPathFrom := filepath.Join(etcdDir, from)
	dataPathTo := filepath.Join(etcdDir, to)
	backupPath := filepath.Join(etcdDir, "backup.json")

	assertNoErr(os.MkdirAll(dataPathFrom, 0777))
	assertNoErr(os.MkdirAll(dataPathTo, 0777))

	c := etcd.Config{
		DataDir:       dataPathFrom,
		Port:          etcdPort,
		Version:       from,
		ContainerName: etcdTestContainerName,
		Image:         etcdImage,
	}
	logrus.Info("Starting etcd")
	assertNoErr(c.Start(context.TODO()))

	logrus.Info("Waiting for etcd to become healthy")
	assertNoErr(waitEtcdHealthy(context.TODO(), etcdPort))

	logrus.Info("Setting up watcher")
	watcher := newWatcher()
	assertNoErr(watcher.test())

	logrus.Info("Writing test data to etcd")
	err = writeEtcdTestData()
	if err != nil {
		return trace.Wrap(err)
	}

	logrus.Info("re-checking the watch")
	assertNoErr(watcher.test())

	logrus.Info("backing up the etcd cluster")
	writer, err := os.Create(backupPath)
	assertNoErr(err)
	backupConf := backup.BackupConfig{
		EtcdConfig: etcdconf.Config{
			Endpoints: []string{fmt.Sprintf("http://127.0.0.1:%v", etcdPort)},
		},
		Prefix: []string{""},
		Writer: writer,
		Log:    logrus.StandardLogger(),
	}
	assertNoErr(backup.Backup(context.TODO(), backupConf))
	assertNoErr(writer.Close())

	logrus.Info("Stopping etcd for upgrade")
	assertNoErr(c.Stop(context.TODO()))

	c = etcd.Config{
		DataDir:       dataPathTo,
		Port:          etcdUpgradePort,
		Version:       from,
		ContainerName: etcdTestContainerName,
		Image:         etcdImage,
	}
	logrus.Info("Starting temporary etcd cluster to initialize database")
	assertNoErr(c.Start(context.TODO()))

	logrus.Info("Waiting for etcd to become healthy")
	assertNoErr(waitEtcdHealthy(context.TODO(), etcdUpgradePort))

	logrus.Info("Stopping temporary etcd cluster")
	assertNoErr(c.Stop(context.TODO()))

	logrus.Info("Running offline restore against etcd DB")
	restoreConf := backup.RestoreConfig{
		File: backupPath,
		Log:  logrus.StandardLogger(),
	}
	assertNoErr(backup.OfflineRestore(context.TODO(), restoreConf, dataPathTo))

	c = etcd.Config{
		DataDir:       dataPathTo,
		Port:          etcdUpgradePort,
		Version:       from,
		ContainerName: etcdTestContainerName,
		Image:         etcdImage,
	}
	logrus.Info("Starting temporary etcd cluster to restore backup")
	assertNoErr(c.Start(context.TODO()))

	logrus.Info("Waiting for etcd to become healthy")
	assertNoErr(waitEtcdHealthy(context.TODO(), etcdUpgradePort))

	logrus.Info("Restoring V2 etcd data and migrating v2 keys to v3")
	restoreConf = backup.RestoreConfig{
		EtcdConfig: etcdconf.Config{
			Endpoints: []string{fmt.Sprintf("http://127.0.0.1:%v", etcdUpgradePort)},
		},
		Prefix:        []string{"/"},        // Restore all etcd data
		MigratePrefix: []string{"/migrate"}, // migrate kubernetes data to etcd3 datastore
		File:          backupPath,
		SkipV3:        true,
		Log:           logrus.StandardLogger(),
	}
	assertNoErr(backup.Restore(context.TODO(), restoreConf))

	logrus.Info("Stopping temporary etcd cluster")
	assertNoErr(c.Stop(context.TODO()))

	c = etcd.Config{
		DataDir:       dataPathTo,
		Port:          etcdPort,
		Version:       from,
		ContainerName: etcdTestContainerName,
		Image:         etcdImage,
	}
	logrus.Info("Starting etcd cluster as new version")
	assertNoErr(c.Start(context.TODO()))

	logrus.Info("Waiting for etcd to become healthy")
	assertNoErr(waitEtcdHealthy(context.TODO(), etcdPort))

	logrus.Info("re-checking the watch")
	assertNoErr(watcher.test())

	logrus.Info("Validating all data exists / is expected value")
	assertNoErr(validateEtcdTestData())

	logrus.Info("re-checking the watch")
	assertNoErr(watcher.test())

	logrus.Info("shutting down etcd, test complete")
	assertNoErr(c.Stop(context.TODO()))

	return nil
}

type watcher struct {
	v2               clientv2.Watcher
	v3               clientv3.WatchChan
	lastSeenRevision int64
}

func newWatcher() watcher {
	cv2, cv3 := etcd.GetClients(etcdPort)
	kapi := clientv2.NewKeysAPI(*cv2)

	return watcher{
		v3: cv3.Watch(context.TODO(), "v3-watch"),
		v2: kapi.Watcher("/v2-watch", &clientv2.WatcherOptions{}),
	}
}

func (w *watcher) test() error {
	cv3, err := etcd.GetClientV3(etcdPort)
	if err != nil {
		return trace.Wrap(err)
	}
	value := fmt.Sprint(rand.Uint64())

	// trigger v3 watch
	resp, err := cv3.Put(context.TODO(), "v3-watch", value)
	if err != nil {
		return trace.Wrap(err)
	}
	logrus.WithFields(logrus.Fields{
		"last_seen_revision": w.lastSeenRevision,
		"put_revision":       resp.Header.GetRevision(),
	}).Info("put v3-watch")

	// check that watch was triggered
	ev := <-w.v3
	w.lastSeenRevision = ev.Header.GetRevision()
	if string(ev.Events[0].Kv.Key) != "v3-watch" || string(ev.Events[0].Kv.Value) != value {
		return trace.BadParameter("Unexpected v3 watcher event, value or key doesn't match expected value").
			AddFields(map[string]interface{}{
				"key":   string(ev.Events[0].Kv.Key),
				"value": string(ev.Events[0].Kv.Value),
			})
	}
	logrus.Info("v3 watch is good.",
		" Key: ", string(ev.Events[0].Kv.Key),
		" Revision: ", ev.CompactRevision,
		" Header.Revision: ", ev.Header.GetRevision(),
	)

	return nil
}

const numKeys = 4000
const numWritesPerKey = 3

// create a sample set of data so that the revision index moves sufficiently forward
func writeEtcdTestData() error {
	cv2, cv3 := etcd.GetClients(etcdPort)
	kapi := clientv2.NewKeysAPI(*cv2)

	// etcdv2
	// create test keys
	for i := 1; i <= numKeys; i++ {
		// write to each key more than once
		for k := 1; k <= numWritesPerKey; k++ {
			_, err := kapi.Set(context.TODO(), fmt.Sprintf("/etcdv2/%v", i), fmt.Sprintf("%v:%v", i, k), nil)
			if err != nil {
				return trace.Wrap(err)
			}
		}
	}

	// migration keys
	// these are keys that should be migrated from the v2 to v3 store during an upgrade
	for i := 1; i <= numKeys; i++ {
		// write to each key more than once
		for k := 1; k <= numWritesPerKey; k++ {
			_, err := kapi.Set(context.TODO(), fmt.Sprintf("/migrate/%v", i), fmt.Sprintf("%v:%v", i, k), nil)
			if err != nil {
				return trace.Wrap(err)
			}
		}
	}

	// etcd v3
	// create 10k keys
	for i := 1; i <= numKeys; i++ {
		// write to each key more than once
		for k := 1; k <= numWritesPerKey; k++ {
			_, err := cv3.Put(context.TODO(), fmt.Sprintf("/etcdv3/%v", i), fmt.Sprintf("%v:%v", i, k))
			if err != nil {
				return trace.Wrap(err)
			}
		}
	}

	return nil
}

// validateEtcdTestData checks all the expected keys exist after the upgrade
func validateEtcdTestData() error {
	cv2, cv3 := etcd.GetClients(etcdPort)
	kapi := clientv2.NewKeysAPI(*cv2)

	// etcdv2
	logrus.Info("validating etcdv2")
	for i := 1; i <= numKeys; i++ {
		// write to each key more than once

		key := fmt.Sprintf("/etcdv2/%v", i)
		resp, err := kapi.Get(context.TODO(), key, &clientv2.GetOptions{})
		if err != nil {
			return trace.Wrap(err)
		}

		expected := fmt.Sprintf("%v:%v", i, numWritesPerKey)
		if resp.Node.Value != expected {
			return trace.BadParameter("unexpected value for key").AddFields(map[string]interface{}{
				"key":      key,
				"expected": expected,
				"value":    resp.Node.Value,
			})
		}

	}

	// migration keys
	// these are keys that should be migrated from the v2 to v3 store during an upgrade
	logrus.Info("validating migration from v2 to v3")
	for i := 1; i <= numKeys; i++ {
		// write to each key more than once

		key := fmt.Sprintf("/migrate/%v", i)
		resp, err := cv3.Get(context.TODO(), key)
		if err != nil {
			return trace.Wrap(err)
		}

		expected := fmt.Sprintf("%v:%v", i, numWritesPerKey)
		if len(resp.Kvs) != 1 {
			return trace.BadParameter("expected to retrieve a key").AddFields(map[string]interface{}{
				"key":      key,
				"expected": expected,
			})
		}
		if string(resp.Kvs[0].Value) != expected {
			return trace.BadParameter("unexpected value for key").AddFields(map[string]interface{}{
				"key":      key,
				"expected": expected,
				"value":    string(resp.Kvs[0].Value),
			})
		}
	}

	// etcd v3
	logrus.Info("validating etcdv3")
	for i := 1; i <= numKeys; i++ {
		// write to each key more than once

		key := fmt.Sprintf("/etcdv3/%v", i)
		resp, err := cv3.Get(context.TODO(), key)
		if err != nil {
			return trace.Wrap(err)
		}

		expected := fmt.Sprintf("%v:%v", i, numWritesPerKey)
		if string(resp.Kvs[0].Value) != expected {
			return trace.BadParameter("unexpected value for key").AddFields(map[string]interface{}{
				"key":      key,
				"expected": expected,
				"value":    string(resp.Kvs[0].Value),
			})
		}

	}

	return nil
}

func waitEtcdHealthy(ctx context.Context, port string) error {
	cv2, _ := etcd.GetClients(etcdPort)
	mapi := clientv2.NewMembersAPI(*cv2)
	for {
		leader, _ := mapi.Leader(ctx)
		if leader != nil {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
}

func assertNoErr(err error) {
	if err != nil {
		panic(trace.DebugReport(err))
	}
}
