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
	"io"
	"io/ioutil"
	"log"
	"os"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/gravitational/trace"
)

const image = "gcr.io/etcd-development/etcd:v3.3.15"
const containerId = "etcd-test-0"

func main() {
	etcdDir, err := ioutil.TempDir("", "etcd")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(etcdDir)
	fmt.Println("Temp etcd directory: ", etcdDir)

	err = startEtcd(etcdDir)
	if err != nil {
		panic(err)
	}
	defer stopEtcd()

	fmt.Println("Etcd Started")

	c := etcdClient()
	err = waitEtcdHealthy(context.TODO(), c)
	if err != nil {
		log.Fatal(trace.DebugReport(err))
	}
	fmt.Println("Connected to etcd. Generating traffic.")
	err = etcdGenerateTraffic(c)
	if err != nil {
		fmt.Println(trace.DebugReport(err))
	}

	fmt.Println("Creating watch")
	/*watcher := etcdWatch(c)
	go func() {
		for {
			fmt.Println("Requesting next watch")
			resp, err := watcher.Next(context.TODO())
			if err != nil {
				log.Println(trace.DebugReport(err))
			}
			spew.Dump(resp)
		}
	}()
	time.Sleep(1 * time.Second)
	*/
	fmt.Println("Triggering watch")
	err = triggerWatch(c)
	if err != nil {
		log.Fatal(trace.DebugReport(err))
	}

	/*fmt.Println("Upgrading DB")
	stopEtcd()
	os.RemoveAll(filepath.Join(etcdDir, "member"))
	err = startEtcd(etcdDir)
	if err != nil {
		log.Fatal(trace.DebugReport(err))
	}
	*/

	fmt.Println("Waiting for etcd")
	err = waitEtcdHealthy(context.TODO(), c)
	if err != nil {
		log.Fatal(trace.DebugReport(err))
	}

	fmt.Println("Triggering watch")
	err = triggerWatch(c)
	if err != nil {
		log.Fatal(trace.DebugReport(err))
	}

	fmt.Println("Updating Index")
	stopEtcd()
	exploreDB(etcdDir)

	fmt.Println("Sleeping")
	time.Sleep(30 * time.Second)
}

func startEtcd(dir string) error {
	cli := dockerClient()

	reader, err := cli.ImagePull(context.TODO(), image, types.ImagePullOptions{})
	if err != nil {
		panic(err)
	}
	io.Copy(os.Stdout, reader)

	etcdClientPort, err := nat.NewPort("tcp", "2379")
	if err != nil {
		return trace.Wrap(err)
	}

	cont, err := cli.ContainerCreate(context.TODO(),
		&container.Config{
			User:  fmt.Sprint(os.Getuid()),
			Image: image,
			Cmd: []string{"/usr/local/bin/etcd",
				"--data-dir", "/etcd-data",
				"--listen-client-urls", "http://0.0.0.0:2379",
				"--advertise-client-urls", "http://127.0.0.1:2379",
				"--snapshot-count", "100",
			},
		},
		&container.HostConfig{
			AutoRemove: true,
			PortBindings: nat.PortMap{
				etcdClientPort: []nat.PortBinding{
					nat.PortBinding{
						HostIP:   "0.0.0.0",
						HostPort: "2379",
					},
				},
			},
			Mounts: []mount.Mount{
				{
					Type:   mount.TypeBind,
					Source: dir,
					Target: "/etcd-data",
				},
			},
		},
		&network.NetworkingConfig{},
		containerId)
	if err != nil {
		return trace.Wrap(err)
	}

	cli.ContainerStart(context.TODO(), cont.ID, types.ContainerStartOptions{})
	fmt.Printf("Container %s is started\n", cont.ID)
	return nil
}

func stopEtcd() {
	cli := dockerClient()
	t := 15 * time.Second
	err := cli.ContainerStop(context.TODO(), containerId, &t)
	if err != nil {
		panic(err)
	}
}

func dockerClient() *client.Client {
	cli, err := client.NewEnvClient()
	if err != nil {
		panic(err)
	}
	cli.NegotiateAPIVersion(context.TODO())
	return cli
}
