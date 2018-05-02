package etcd

import (
	"context"
	"encoding/json"
	"os"

	etcdv2 "github.com/coreos/etcd/client"
	etcdv3 "github.com/coreos/etcd/clientv3"
	etcdconf "github.com/gravitational/coordinate/config"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

// BackupConfig are the settings to use for running a backup of the etcd database
type BackupConfig struct {
	EtcdConfig etcdconf.Config
	Prefix     []string
	File       string
	Log        log.FieldLogger
}

func (b *BackupConfig) CheckAndSetDefaults() error {
	if b.Prefix == nil {
		b.Prefix = []string{"/"}
	}
	return nil
}

func Backup(ctx context.Context, conf BackupConfig) error {
	err := conf.CheckAndSetDefaults()
	if err != nil {
		return trace.Wrap(err)
	}

	conf.Log.Info("Starting etcd backup.")
	keysv2, clientv3, err := getClients(conf.EtcdConfig)
	if err != nil {
		return trace.Wrap(err)
	}
	defer clientv3.Close()

	file, err := os.OpenFile(conf.File, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return trace.Wrap(err)
	}
	defer file.Close()

	enc := json.NewEncoder(file)
	enc.Encode(&backupVersion{Version: FileVersion})

	for _, prefix := range conf.Prefix {
		err = runV2Backup(ctx, enc, keysv2, prefix)
		if err != nil {
			return trace.Wrap(err)
		}

		err = runv3Backup(ctx, enc, clientv3, prefix)
		if err != nil {
			return trace.Wrap(err)
		}
	}

	conf.Log.Info("Completed etcd backup.")

	return nil
}

func runV2Backup(ctx context.Context, w *json.Encoder, client etcdv2.KeysAPI, prefix string) error {
	// Note, this may need to be revisited in the future
	// We query the entire etcd database in a single query, which may cause excessive memory usage
	res, err := client.Get(ctx, prefix, &etcdv2.GetOptions{Sort: true, Recursive: true})
	if err != nil {
		return err
	}

	err = recurseV2Nodes(ctx, w, res.Node)
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}

func recurseV2Nodes(ctx context.Context, w *json.Encoder, n *etcdv2.Node) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		nodes := n.Nodes
		n.Nodes = nil

		err := w.Encode(etcdBackupNode{
			V2: n,
		})
		if err != nil {
			return trace.Wrap(err)
		}

		if nodes != nil {
			for _, node := range nodes {
				err = recurseV2Nodes(ctx, w, node)
				if err != nil {
					return trace.Wrap(err)
				}
			}
		}
	}

	return nil
}

func runv3Backup(ctx context.Context, w *json.Encoder, client *etcdv3.Client, prefix string) error {
	// Note, this may need to be revisited in the future
	// We query the entire etcd database in a single query, which may cause excessive memory usage
	resp, err := client.KV.Get(ctx, prefix, etcdv3.WithPrefix())
	if err != nil {
		return trace.Wrap(err)
	}

	for _, ev := range resp.Kvs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var ttl int64
		if ev.Lease != 0 {
			lease, err := client.Lease.TimeToLive(ctx, etcdv3.LeaseID(ev.Lease))
			if err != nil {
				return trace.Wrap(err)
			}
			ttl = lease.TTL
		}

		err := w.Encode(etcdBackupNode{
			V3: &KeyValue{
				Key:            ev.Key,
				CreateRevision: ev.CreateRevision,
				ModRevision:    ev.ModRevision,
				Version:        ev.Version,
				Value:          ev.Value,
				Lease:          ev.Lease,
				TTL:            ttl,
			},
		})
		if err != nil {
			return trace.Wrap(err)
		}
	}

	return nil
}
