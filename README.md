# Planet

Planet is a containerized [Kubernetes](https://kubernetes.io/) environment. It is a self-containerizing Debian image with
Kubernetes services running inside. [Gravity](https://github.com/gravitational/gravity) is the
recommended way to deploy planet containers.

Compared to [official ways](https://kubernetes.io/docs/setup/) to install and manage a Kubernetes cluster, `planet` is different because:

* Planet creates a "bubble of consistency" for every cluster.
* Planet packages services running under/alongside Kubernetes.
* Planet facilitates easier remote updating of itself and Kubernetes.

## Installation

Planet images are automatically packaged by [Gravity](https://github.com/gravitational/gravity) - this is the easiest way to use and hack on planet.
See the development section below for details on how to build & hack on planet locally.

## Details of Operation

Planet is a generic `container image` with executable entry points -- it is an archived [root filesystem](http://www.tldp.org/LDP/sag/html/root-fs.html)
Planet uses [libcontainer](https://github.com/opencontainers/runc/tree/master/libcontainer) to self-containerize.
Planet launches [systemd](http://www.freedesktop.org/wiki/Software/systemd/) inside the container as the main process which manages the
lifetime of all the other services - [Kubernetes](https://github.com/kubernetes/kubernetes), among others.
A `planet` binary is available within planet as `rootfs/usr/bin/planet`

That `planet` binary defines all entry points for this package. Here is a brief summary of the planet interface:
```
Commands:
  help [<command>...]
    Show help.

  version
    Print version information

  start [<flags>]
    Start Planet container

  agent --leader-key=LEADER-KEY --election-key=ELECTION-KEY [<flags>]
    Start Planet Agent

  stop
    Stop planet container

  enter [<flags>] [<cmd>]
    Enter running planet container

  status [<flags>]
    Query the planet cluster status

  test --kube-addr=KUBE-ADDR [<flags>]
    Run end-to-end tests on a running cluster

  device add --data=DATA
    Add new device to container

  device remove --node=NODE
    Remove device from container

  etcd promote --name=NAME --initial-cluster=INITIAL-CLUSTER --initial-cluster-state=INITIAL-CLUSTER-STATE
    Promote etcd running in proxy mode to a full member

  leader pause
    Pause leader election participation for this node

  leader resume
    Resume leader election participation for this node

  leader view --leader-key=LEADER-KEY
    Display the IP address of the active master
```

## Hacking on Planet

We follow a [Code of Conduct](./CODE_OF_CONDUCT.md). We also have [contributing guidelines](./CONTRIBUTING.md)
with information about filing bugs and submitting patches.

### Building (installing from source)

Prerequisites for planet development are:
 - [docker](https://docker.com/) version >= 1.8.2 is required. For development, you need to be
   inside docker group and have the docker daemon running so the typical docker commands like `docker run`
   do not require sudo permissions. Here's official [docker configuration](https://docs.docker.com/engine/articles/configuring/) reference.
 - (optional) [vagrant](https://www.vagrantup.com/) version >= 1.7.4

The building process has been tested on `Debian 8` and `Ubuntu 15.04`.

The output of Planet build is a tarball that goes into `build/$TARGET`:

Following are the most common targets:

 - `make production` - builds a planet images. These are the images used by Gravity.

Building planet for the first time takes considerable amount of time since it has to download/build/configure
quite a few dependencies:
 - Kubernetes
 - docker registry
 - [flannel](https://github.com/coreos/flannel>)
 - [etcd](https://github.com/coreos/etcd)
 - [serf](https://github.com/hashicorp/serf)

Subsequent builds are much faster because intermediate results are cached (in `build/assets` directory).
To clear and rebuild from scratch, run the `make clean`.

### Upgrading dependencies

If your patch depends on new go packages, the dependencies must:

- be licensed via Apache2 license
- be approved by core Planet contributors ahead of time
- be vendored via go modules

Non-go (root fs) dependencies are encapsulated into their own build packages rooted
at `build.assets`.

For instance, to upgrade etcd, edit build.assets/makefiles/etcd/etcd.mk:

```Makefile
...
VER := v2.2.3
...
```

The version of Kubernetes is defined in the root makefile:

```Makefile
...
KUBE_VER := v1.17.9
...
```

## Planet Design

### Planet Daemon

Once started, planet will continue running and waiting for the main systemd process to end.
The container environment is encapsulated by the [box](https://github.com/gravitational/planet/tree/master/lib/box) package.
As part of its operation, planet will also start a web server to enable remote process control.
The server listens on the unix socket (in a `/var/run/planet.sock` by default) and is capable of
running commands inside the container on behalf of the client - this is how the commands `planet stop`
and `planet enter` are implemented.

### Planet Agent

One additional service is the agent. Agent is responsible for maintaining the health status
of the cluster (obtainable with `planet status`) and implementing the master fail-over by
dynamically promoting nodes to master should the active master fail.
