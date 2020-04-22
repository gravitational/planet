package etcd

import (
	"context"
	"fmt"
	"io"
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
	"github.com/sirupsen/logrus"
	clientv2 "go.etcd.io/etcd/client"
	"go.etcd.io/etcd/clientv3"
)

// Config defines etcd connect configuration
type Config struct {
	DataDir         string
	ForceNewCluster bool
	Port            string
	Version         string
	ContainerName   string
	Image           string
}

// Start starts a new etcd container with the specified configuration
func (r *Config) Start(ctx context.Context) error {
	cli := dockerClient(ctx)

	// Check if a container already exists, and if it does, clean up the container
	containers, err := cli.ContainerList(ctx, types.ContainerListOptions{All: true})
	if err != nil {
		return trace.Wrap(err)
	}
	for _, container := range containers {
		if len(container.Names) > 0 && container.Names[0] == fmt.Sprintf("/%v", r.ContainerName) {
			logrus.Info("Stopping Container ID: ", container.ID)
			_ = cli.ContainerStop(ctx, container.ID, nil)
			_ = cli.ContainerRemove(ctx, container.ID, types.ContainerRemoveOptions{})
		}
	}

	// Pull the image from the upstream repository
	image := fmt.Sprintf("%v:%v", r.Image, r.Version)
	reader, err := cli.ImagePull(ctx, image, types.ImagePullOptions{})
	if err != nil {
		return trace.Wrap(err)
	}
	io.Copy(os.Stdout, reader)

	etcdClientPort, err := nat.NewPort("tcp", "2379")
	if err != nil {
		return trace.Wrap(err)
	}

	cmd := []string{"/usr/local/bin/etcd",
		"--data-dir", "/etcd-data",
		"--enable-v2=true",
		"--listen-client-urls", "http://0.0.0.0:2379",
		"--advertise-client-urls", "http://127.0.0.1:2379",
		"--snapshot-count", "100",
		"--initial-cluster-state", "new",
	}
	if r.ForceNewCluster {
		cmd = append(cmd, "--force-new-cluster")
	}

	cont, err := cli.ContainerCreate(ctx,
		&container.Config{
			User:  fmt.Sprint(os.Getuid()),
			Image: image,
			Cmd:   cmd,
		},
		&container.HostConfig{
			PortBindings: nat.PortMap{
				etcdClientPort: []nat.PortBinding{
					nat.PortBinding{
						HostIP:   "127.0.0.1",
						HostPort: r.Port,
					},
				},
			},
			Mounts: []mount.Mount{
				{
					Type:   mount.TypeBind,
					Source: r.DataDir,
					Target: "/etcd-data",
				},
			},
		},
		&network.NetworkingConfig{},
		r.ContainerName)
	if err != nil {
		return trace.Wrap(err)
	}

	err = cli.ContainerStart(ctx, cont.ID, types.ContainerStartOptions{})
	if err != nil {
		return trace.Wrap(err)
	}
	logrus.Printf("Container %s (%s) is started\n", r.ContainerName, cont.ID)

	return nil
}

// Stop stops the container
func (r *Config) Stop(ctx context.Context) error {
	cli := dockerClient(ctx)
	timeout := 15 * time.Second
	logrus.Info("Stopping Container ID: ", r.ContainerName)
	return trace.Wrap(cli.ContainerStop(ctx, r.ContainerName, &timeout))
}

// GetClients creates etcd clients of both versions
func GetClients(port string) (*clientv2.Client, *clientv3.Client) {
	cfg := clientv2.Config{
		Endpoints: []string{fmt.Sprintf("http://127.0.0.1:%v", port)},
		Transport: clientv2.DefaultTransport,
		// set timeout per request to fail fast when the target endpoint is unavailable
		HeaderTimeoutPerRequest: time.Second,
	}
	clientv2, err := clientv2.New(cfg)
	if err != nil {
		log.Fatal(err)
	}

	clientv3, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{fmt.Sprintf("http://127.0.0.1:%v", port)},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		log.Fatal(err)
	}

	return &clientv2, clientv3
}

func GetClientV3(port string) (*clientv3.Client, error) {
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{fmt.Sprintf("http://127.0.0.1:%v", port)},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return client, nil
}

func dockerClient(ctx context.Context) *client.Client {
	cli, err := client.NewEnvClient()
	if err != nil {
		panic(err)
	}
	cli.NegotiateAPIVersion(ctx)
	return cli
}
