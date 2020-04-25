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
	"time"

	etcdv2 "github.com/coreos/etcd/client"
	etcdv3 "github.com/coreos/etcd/clientv3"
	"github.com/coreos/go-semver/semver"
	etcdconf "github.com/gravitational/coordinate/config"
	"github.com/gravitational/trace"
)

func getClients(config etcdconf.Config) (etcdv2.KeysAPI, *etcdv3.Client, error) {
	err := config.CheckAndSetDefaults()
	if err != nil {
		return nil, nil, trace.Wrap(err)
	}

	clientv2, err := config.NewClient()
	if err != nil {
		return nil, nil, trace.Wrap(err)
	}
	keysv2 := etcdv2.NewKeysAPI(clientv2)

	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	version, err := clientv2.GetVersion(ctx)
	if err != nil {
		return nil, nil, trace.Wrap(err)
	}
	serverVersion := semver.New(version.Server)
	v3 := semver.New("3.0.0")
	// if we're talking to a v2 only etcd server, don't try and use the v3 client
	if serverVersion.LessThan(*v3) {
		return keysv2, nil, nil
	}

	clientv3, err := config.NewClientV3()
	if err != nil {
		return nil, nil, trace.Wrap(err)
	}

	return keysv2, clientv3, nil
}

type etcdBackupNode struct {
	V2 *etcdv2.Node `json:"v2,omitempty"`
	V3 *KeyValue    `json:"v3,omitempty"`
}

// store information in the backup about the version fo the backup
type backupVersion struct {
	Version string `json:"version"`
}

// KeyValue is a clone of the internal KeyValue from etcd which isn't exported
type KeyValue struct {
	// key is the key in bytes. An empty key is not allowed.
	Key []byte `protobuf:"bytes,1,opt,name=key,proto3" json:"key,omitempty"`
	// create_revision is the revision of last creation on this key.
	CreateRevision int64 `protobuf:"varint,2,opt,name=create_revision,json=createRevision,proto3" json:"create_revision,omitempty"`
	// mod_revision is the revision of last modification on this key.
	ModRevision int64 `protobuf:"varint,3,opt,name=mod_revision,json=modRevision,proto3" json:"mod_revision,omitempty"`
	// version is the version of the key. A deletion resets
	// the version to zero and any modification of the key
	// increases its version.
	Version int64 `protobuf:"varint,4,opt,name=version,proto3" json:"version,omitempty"`
	// value is the value held by the key, in bytes.
	Value []byte `protobuf:"bytes,5,opt,name=value,proto3" json:"value,omitempty"`
	// lease is the ID of the lease that attached to key.
	// When the attached lease expires, the key will be deleted.
	// If lease is 0, then no lease is attached to the key.
	Lease int64 `protobuf:"varint,6,opt,name=lease,proto3" json:"lease,omitempty"`
	// TTL (not from etcd datastructure)
	// This is the TTL of the key, which we look up during the backup, because etcd3 stores these separatly from the key
	TTL int64 `json:"ttl,omitempty"`
}
