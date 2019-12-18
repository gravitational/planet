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
	"os"
	"path/filepath"
	"time"

	"github.com/gravitational/trace"
	bolt "go.etcd.io/bbolt"
	"go.etcd.io/etcd/client"
	"go.etcd.io/etcd/clientv3"
	"go.etcd.io/etcd/mvcc"
	"go.etcd.io/etcd/mvcc/backend"
	"go.etcd.io/etcd/mvcc/mvccpb"
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

func etcd3Client() *clientv3.Client {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{"http://localhost:2379"},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		log.Fatal(err)
	}

	_, err = cli.Put(context.TODO(), "foo", "bar")
	if err != nil {
		log.Fatal(err)
	}

	return cli
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

func etcdv3Write100Keys(c *clientv3.Client) error {
	for i := 1; i <= 1000; i++ {
		_, err := c.Put(context.TODO(), fmt.Sprint("etcd3-", i), fmt.Sprint(i))
		if err != nil {
			return trace.Wrap(err)
		}

		//spew.Dump(resp)
	}
	return nil
}

func etcdGenerateTraffic(c client.Client) error {
	kapi := client.NewKeysAPI(c)

	for k := 1; k <= 1000; k++ {
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
	var be backend.Backend

	time.Sleep(1 * time.Second)

	bch := make(chan struct{})
	dbpath := filepath.Join(dir, "member", "snap", "db")
	fmt.Println("Dbpath: ", dbpath)
	go func() {
		defer close(bch)
		be = backend.NewDefaultBackend(dbpath)
	}()

	select {
	case <-bch:
	case <-time.After(time.Second):
		fmt.Fprintf(os.Stderr, "waiting for etcd to close and release its lock on %q\n", dbpath)
		<-bch
	}
	defer be.Close()

	fmt.Println("BatchTx")
	tx := be.BatchTx()
	tx.Lock()
	tx.UnsafeCreateBucket([]byte("key"))
	tx.UnsafeCreateBucket([]byte("meta"))
	tx.Unlock()

	fmt.Println("WriteKV")
	mvcc.WriteKV(be, mvccpb.KeyValue{
		Key:            []byte("test-999"),
		CreateRevision: 10 * 1000 * 999,
		ModRevision:    10 * 1000 * 999,
		Value:          []byte("test-999"),
		Version:        1,
	})
	mvcc.WriteKV(be, mvccpb.KeyValue{
		Key:            []byte("test"),
		CreateRevision: 10 * 1000 * 1000,
		ModRevision:    10 * 1000 * 1000,
		Value:          []byte("test"),
		Version:        1,
	})

	fmt.Println("UpdateConsistentIndex")
	//mvcc.UpdateConsistentIndex(be, 10*1000*1000)

	fmt.Println("Read Consistent Index")
	//_, vs := tx.UnsafeRange([]byte("meta"), []byte("consistent_index"), nil, 0)
	//v := binary.BigEndian.Uint64(vs[0])
	//fmt.Println("consistent_index: ", v)
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
