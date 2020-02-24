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

package etcd

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cenkalti/backoff"

	"go.etcd.io/etcd/mvcc"
	"go.etcd.io/etcd/mvcc/backend"
	"go.etcd.io/etcd/mvcc/mvccpb"

	etcdconf "github.com/gravitational/coordinate/config"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
	etcdv2 "go.etcd.io/etcd/client"
	etcdv3 "go.etcd.io/etcd/clientv3"
)

// BackupConfig are the settings to use for running a backup of the etcd database
type RestoreConfig struct {
	EtcdConfig    etcdconf.Config
	Prefix        []string
	MigratePrefix []string
	File          string
	MinRestoreTTL time.Duration
	Log           log.FieldLogger
	SkipV3        bool
}

func (b *RestoreConfig) CheckAndSetDefaults() error {
	if b.Prefix == nil {
		b.Prefix = []string{"/"}
	}

	// Etcd has a hard coded minimum TTL, so don't try to restore key's that are about to expire anyway's
	if b.MinRestoreTTL < DefaultMinRestoreTTL {
		b.MinRestoreTTL = DefaultMinRestoreTTL
	}

	return nil
}

// Restore restores a backup to an etcd database using the etcd API (does not preserve revision or index numbers)
func Restore(ctx context.Context, conf RestoreConfig) error {
	err := conf.CheckAndSetDefaults()
	if err != nil {
		return trace.Wrap(err)
	}

	conf.Log.Info("Starting etcd restore.")
	keysv2, clientv3, err := getClients(conf.EtcdConfig)
	if err != nil {
		return trace.Wrap(err)
	}

	file, err := os.Open(conf.File)
	if err != nil {
		return trace.Wrap(err)
	}

	decoder := json.NewDecoder(file)

	// The first record in the backup file should be the version
	var version backupVersion
	err = decoder.Decode(&version)
	if err != nil {
		return trace.Wrap(err)
	}
	if version.Version != FileVersion {
		return trace.BadParameter("Unsupported backup version %v. ", version.Version)
	}

	for {
		var node etcdBackupNode
		err := decoder.Decode(&node)
		if err == io.EOF {
			break
		} else if err != nil {
			return trace.Wrap(err)
		}

		if node.V2 != nil {
			err = restoreNodeV2(ctx, conf, node.V2, keysv2, clientv3)
			if err != nil {
				return err
			}
		}
		if !conf.SkipV3 && node.V3 != nil {
			err = restoreNodeV3(ctx, conf, node.V3, clientv3)
			if err != nil {
				return err
			}
		}
	}

	conf.Log.Info("Completed etcd restore.")

	return nil
}

// OfflineRestore restores the etcdv3 database to an offline etcd nodes snapshot DB.
func OfflineRestore(ctx context.Context, conf RestoreConfig, dir string) error {
	err := conf.CheckAndSetDefaults()
	if err != nil {
		return trace.Wrap(err)
	}

	conf.Log.Info("Starting etcd offline restore.")

	file, err := os.Open(conf.File)
	if err != nil {
		return trace.Wrap(err)
	}

	decoder := json.NewDecoder(file)

	// The first record in the backup file should be the version
	var version backupVersion
	err = decoder.Decode(&version)
	if err != nil {
		return trace.Wrap(err)
	}
	if version.Version != FileVersion {
		return trace.BadParameter("Unsupported backup version %v. ", version.Version)
	}

	// Open the etcd database directly
	var be backend.Backend
	bch := make(chan struct{})
	go func() {
		defer close(bch)
		be = backend.NewDefaultBackend(filepath.Join(dir, "member", "snap", "db"))
	}()
	select {
	case <-bch:
	case <-time.After(30 * time.Second):
		return trace.BadParameter("timed out waiting for etcd db lock").AddField("dir", dir)
	}
	defer be.Close()

	// Restore the DB
	for {
		var node etcdBackupNode
		err := decoder.Decode(&node)
		if err == io.EOF {
			break
		} else if err != nil {
			return trace.Wrap(err)
		}

		if node.V3 != nil {
			writeV3KV(be, node.V3)
		}
	}

	conf.Log.Info("Completed etcd offline restore.")

	return nil
}

func writeV3KV(be backend.Backend, node *KeyValue) {
	mvcc.WriteKV(be, mvccpb.KeyValue{
		Key:            node.Key,
		CreateRevision: node.CreateRevision,
		ModRevision:    node.ModRevision,
		Value:          node.Value,
		Version:        node.Version,
	})
}

func restoreNodeV2(ctx context.Context, conf RestoreConfig, node *etcdv2.Node, keysv2 etcdv2.KeysAPI, clientv3 *etcdv3.Client) error {
	if !checkPrefix(node.Key, conf.Prefix) {
		return nil
	}

	// Migrate from v2 to v3
	if checkPrefix(node.Key, conf.MigratePrefix) {

		// V3 datastore doesn't have directories
		if node.Dir {
			return nil
		}

		// convert V2 TTL's to V3 lease if set.
		if node.TTL != 0 {
			// etcdv3 doesn't support a lease of less than 5 seconds
			// so we'll skip restoring key's that were about to expire anyway's.
			if node.TTLDuration() <= conf.MinRestoreTTL {
				conf.Log.Warnf("Skipping restore of key: %v because it's TTL is too short (%v <= %v)", node.Key, node.TTLDuration(), conf.MinRestoreTTL)
				return nil
			}
			lease, err := clientv3.Grant(ctx, node.TTL)
			if err != nil {
				return trace.Wrap(err)
			}

			return trace.Wrap(retry(ctx, func() error {
				_, err := clientv3.KV.Put(ctx, node.Key, node.Value, etcdv3.WithLease(lease.ID))
				if err != nil {
					return trace.Wrap(err).AddField("key", node.Key)
				}
				return nil
			}))

		}
		return trace.Wrap(retry(ctx, func() error {
			_, err := clientv3.KV.Put(ctx, node.Key, node.Value)
			if err != nil {
				return trace.Wrap(err).AddField("key", node.Key)
			}
			return nil
		}))

	}
	return trace.Wrap(retry(ctx, func() error {
		_, err := keysv2.Set(ctx, node.Key, node.Value, &etcdv2.SetOptions{TTL: node.TTLDuration(), Dir: node.Dir})
		if err != nil {
			return trace.Wrap(err).AddField("key", node.Key)
		}
		return nil
	}))

}

func restoreNodeV3(ctx context.Context, conf RestoreConfig, node *KeyValue, clientv3 *etcdv3.Client) error {
	if !checkPrefix(string(node.Key), conf.Prefix) {
		return nil
	}

	if node.TTL != 0 {
		if node.TTL <= int64(conf.MinRestoreTTL.Seconds()) {
			conf.Log.Warnf("Skipping restore of key: %v because it's TTL is too short (%v <= %v)", string(node.Key), node.TTL, conf.MinRestoreTTL)
			return nil
		}
		lease, err := clientv3.Grant(ctx, node.TTL)
		if err != nil {
			return trace.Wrap(err)
		}
		return trace.Wrap(retry(ctx, func() error {
			_, err := clientv3.KV.Put(ctx, string(node.Key), string(node.Value), etcdv3.WithLease(lease.ID))
			if err != nil {
				return trace.Wrap(err).AddField("key", string(node.Key))
			}
			return nil
		}))
	}

	return trace.Wrap(retry(ctx, func() error {
		_, err := clientv3.KV.Put(ctx, string(node.Key), string(node.Value))
		if err != nil {
			return trace.Wrap(err).AddField("key", string(node.Key))
		}
		return nil
	}))

}

const retryMaxElapsedTime = 60 * time.Second

func retry(ctx context.Context, f func() error) error {
	interval := backoff.NewExponentialBackOff()
	interval.MaxElapsedTime = retryMaxElapsedTime
	b := backoff.WithContext(interval, ctx)
	return trace.Wrap(backoff.Retry(f, b))
}

// checkPrefix checks if a given string matches a list of prefixes
func checkPrefix(check string, prefix []string) bool {
	if prefix == nil {
		return false
	}

	for _, p := range prefix {
		if strings.HasPrefix(check, p) {
			return true
		}
	}

	return false
}
