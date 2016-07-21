# Planet

Planet is a containerized Kubernetes environment, it is a self-containerizing Ubuntu 15.04 image with
Kubernetes services running inside. 

There are [official ways] to install and manager a Kubernetes cluster, but `planet` differs from those:

* Planet creates a "bubble of consistency" for every cluster we deploy.
* Planet allows to package our own services running under/alongside Kubernetes.
* Planet facilitates easier remote updating of itself and Kubernetes.

It also happens to be a great way to play with Kubernetes!
Also check out the [developer documentation](docs/README.md).

## Installation

Planet images are automatically packaged by [gravity] - this is the easiest way to use and hack on planet.
See the development section below on details on how to build a planet image for [gravity].

## Details of Operation

Planet is a generic `container image` with executable entry points. Basically, it is an archived [root filesystem].
Inside a container image there are Kubernetes components specific to the image and the planet binary itself as `rootfs/usr/bin/planet`.

That `planet` binary defines all entry points for this package.

When you launch planet via `planet start`, it will self-containerize within its RootFS and start all Kubernetes
components using `[systemd]`.

Here is a brief summary of the planet interface:
```
Commands:
  version
    Print version information

  start [<flags>]
    Start Planet container

  agent --leader-key=LEADER-KEY [<flags>]
    Start Planet Agent

  stop
    Stop planet container

  enter [<flags>] [<cmd>]
    Enter running planet container

  status [<flags>]
    Query the planet cluster status

  test --kube-addr=KUBE-ADDR [<flags>]
    Run end-to-end tests on a running cluster

  secrets init --domain=DOMAIN [<flags>] <dir>
    initialize directory with secrets
```

## Hacking on Planet

### Building (installing from source)

Pprerequisites for planet development are:
 - [docker] version >= 1.8.2 is required. For development, you need to be
   inside docker group and have the docker daemon running so the typical docker commands like `docker run`
   do not require sudo permissions. Here's official [docker configuration] reference.
 - (optional) [vagrant] version >= 1.7.4

The building process has been tested on `Debian 8` and `Ubuntu 15.04`.

The output of Planet build is a tarball that goes into `build/$TARGET`:

Following are the most common targets:

 - `make production` - builds separate master/node planet images. These are the images used by [gravity].
 - `make dev` - builds a combined (master+node) image.

Building planet for the first time takes considerable amount of time since it has to download/build/configure
quite a few dependencies:
 - [kubernetes]
 - docker registry
 - [flannel]
 - [etcd]
 - [serf]

Subsequent builds are much faster because intermediate results are cached (in `build/assets` directory).
To clear and rebuild from scratch, run one of the following (depending on the target):

 - `make node-clean`
 - `make master-clean`
 - `make dev-clean`
 - or `make clean` to wipe everything out

### Upgrading dependencies

Sometimes we need to upgrade to a newer version of the specific dependency. This is relatively simple given
the modular approach to build assets. Most dependencies are encapsulated into their own build packages rooted
at `build.assets`.

For instance, to upgrade etcd, edit build.assets/makefiles/etcd/etcd.mk:

```Makefile
...
VER := v2.2.3
...
```

The version of [kubernetes] is defined in the root makefile:

```Makefile
...
KUBE_VER := v1.1.4
...
```

### Making changes

Although it is still possible to develop with `make dev`, the recommended way is to use [gravity] coupled with
[virsh provisioner] for improved development experience.

### Production Mode

[gravity] is the recommended way to deploy planet containers.

### Working with "dev" image

Run: 

```
make dev-start
```

You will need another terminal to interact with it. To enter a running Planet container, 
execute `make enter`. You can then inspect running components with `ps -e`:

```
  PID TTY          TIME CMD
    1 ?        00:00:00 systemd
   61 ?        00:00:01 systemd-journal
   76 ?        00:00:00 systemd-logind
   79 ?        00:00:00 dbus-daemon
   82 ?        00:00:00 systemd-resolve
 1766 ?        00:00:00 kube-proxy
 2879 ?        00:00:00 bash
 4724 ?        00:00:00 kube-apiserver
 4725 ?        00:00:00 kube-scheduler
 4726 ?        00:00:00 kube-controller
```

To stop, hit `Ctrl+C` or run `make stop` in another terminal.


[//]: # (Footnots and references)

[official ways]: <http://kubernetes.io/v1.1/docs/getting-started-guides/README.html>
[root filesystem]: <http://www.tldp.org/LDP/sag/html/root-fs.html>
[Kubernetes]: <https://github.com/kubernetes/kubernetes>
[systemd]: <http://www.freedesktop.org/wiki/Software/systemd/>
[docker]: <https://docs.docker.com/linux/step_one/>
[docker configuration]: <https://docs.docker.com/engine/articles/configuring/>
[vagrant]: <https://www.vagrantup.com/downloads.html>
[gravity]: <https://github.com/gravitational/gravity>
[flannel]: <https://github.com/coreos/flannel>
[etcd]: <https://github.com/coreos/etcd>
[serf]: <https://github.com/hashicorp/serf>
[virsh provisioner]: <https://github.com/gravitational/gravity/tree/master/assets/virsh-centos>
