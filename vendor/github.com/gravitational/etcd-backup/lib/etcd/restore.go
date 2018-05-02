package etcd

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"time"

	etcdv2 "github.com/coreos/etcd/client"
	etcdv3 "github.com/coreos/etcd/clientv3"
	etcdconf "github.com/gravitational/coordinate/config"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

// BackupConfig are the settings to use for running a backup of the etcd database
type RestoreConfig struct {
	EtcdConfig    etcdconf.Config
	Prefix        []string
	MigratePrefix []string
	File          string
	MinRestoreTTL time.Duration
	Log           log.FieldLogger
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
		} else {
			err = restoreNodeV3(ctx, conf, node.V3, clientv3)
			if err != nil {
				return err
			}
		}

	}

	conf.Log.Info("Completed etcd restore.")

	return nil
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
			clientv3.KV.Put(ctx, node.Key, node.Value, etcdv3.WithLease(lease.ID))
		} else {
			clientv3.KV.Put(ctx, node.Key, node.Value)
		}

	} else {
		keysv2.Set(ctx, node.Key, node.Value, &etcdv2.SetOptions{TTL: node.TTLDuration(), Dir: node.Dir})
	}
	return nil
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
		clientv3.KV.Put(ctx, string(node.Key), string(node.Value), etcdv3.WithLease(lease.ID))
	} else {
		clientv3.KV.Put(ctx, string(node.Key), string(node.Value))
	}
	return nil
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
