/*
Copyright 2019 Gravitational, Inc.

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
	"context"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/gravitational/trace"
	bolt "go.etcd.io/bbolt"
	"go.etcd.io/etcd/client"
)

func etcdClient() client.Client {
	cfg := client.Config{
		Endpoints: []string{"http://127.0.0.1:2379"},
		Transport: client.DefaultTransport,
		// set timeout per request to fail fast when the target endpoint is unavailable
		HeaderTimeoutPerRequest: time.Second,
	}
	c, err := client.New(cfg)
	if err != nil {
		log.Fatal(err)
	}

	return c
}

func waitEtcdHealthy(ctx context.Context, c client.Client) error {
	mapi := client.NewMembersAPI(c)
	for {
		leader, _ := mapi.Leader(ctx)
		if leader != nil {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
}

func etcdWrite100Keys(c client.Client) error {
	kapi := client.NewKeysAPI(c)

	for i := 1; i <= 100; i++ {
		_, err := kapi.Set(context.TODO(), fmt.Sprint(i), fmt.Sprint(i), nil)
		if err != nil {
			return trace.Wrap(err)
		}

		//spew.Dump(resp)
	}

	return nil
}

func etcdGenerateTraffic(c client.Client) error {
	kapi := client.NewKeysAPI(c)

	for k := 1; k <= 100; k++ {
		for i := 1; i <= 10; i++ {
			_, err := kapi.Set(context.TODO(), fmt.Sprint(i), fmt.Sprint(i+k), nil)
			if err != nil {
				return trace.Wrap(err)
			}

			//spew.Dump(resp)
		}
	}
	return nil
}

func etcdWatch(c client.Client) client.Watcher {
	kapi := client.NewKeysAPI(c)
	return kapi.Watcher("1", &client.WatcherOptions{})
}

func triggerWatch(c client.Client) error {
	kapi := client.NewKeysAPI(c)
	_, err := kapi.Set(context.TODO(), "1", "z", nil)
	return trace.Wrap(err)
}

func tryJumpEtcdIndex(dir string) {

}

func exploreDB(dir string) error {
	//_ = backend.NewDefaultBackend(filepath.Join(dir, "member/wal/0000000000000000-0000000000000000.wal"))
	bopts := &bolt.Options{
		Timeout:         10 * time.Second,
		ReadOnly:        true,
		InitialMmapSize: 10 * 1024 * 1024 * 1024,
	}

	db, err := bolt.Open(filepath.Join(dir, "member/snap/db"), 0600, bopts)
	if err != nil {
		return trace.Wrap(err)
	}

	err = db.View(func(tx *bolt.Tx) error {
		log.Println("Iterating buckets")
		tx.ForEach(func(name []byte, b *bolt.Bucket) error {
			log.Println("- ", string(name))
			return nil
		})

		log.Println("Iterating keys")
		keys := tx.Bucket([]byte("key"))
		keys.ForEach(func(k, v []byte) error {
			log.Println(string(k), ":", string(v))
			return nil
		})

		return nil
	})
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}
