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

	etcdconf "github.com/gravitational/coordinate/config"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
	etcdv2 "go.etcd.io/etcd/client"
	etcdv3 "go.etcd.io/etcd/clientv3"
)

// BackupConfig are the settings to use for running a backup of the etcd database
type BackupConfig struct {
	EtcdConfig etcdconf.Config
	Prefix     []string
	Writer     io.Writer
	Log        log.FieldLogger
}

func (b *BackupConfig) CheckAndSetDefaults() error {
	if b.Prefix == nil {
		b.Prefix = []string{"/"}
	}
	if b.Writer == nil {
		b.Writer = os.Stdout
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
	if clientv3 != nil {
		defer clientv3.Close()
	}

	enc := json.NewEncoder(conf.Writer)
	enc.Encode(&backupVersion{Version: FileVersion})

	for _, prefix := range conf.Prefix {
		err = runV2Backup(ctx, enc, keysv2, prefix)
		if err != nil {
			return trace.Wrap(err)
		}

		if clientv3 != nil {
			err = runv3Backup(ctx, enc, clientv3, prefix)
			if err != nil {
				return trace.Wrap(err)
			}
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
