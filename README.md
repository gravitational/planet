# Planet

Planet is a containerized Kubernetes environment, it is a self-containerizing Ubuntu 15.04 image with
Kubernetes services running inside. 

There are [official ways](http://kubernetes.io/v1.0/docs/getting-started-guides/README.html) to install and 
play with Kubernetes, but `Planet` differs from those because:

* Planet creates a "bubble of consistency" for every Kubernetes cluster we deploy.
* Planet allows to package our own services running under/alongside Kubernetes.
* Planet facilitates easier remote updating of itself and Kubernetes (because it uses [Orbit containers](https://github.com/gravitational/orbit))

It also happens to be a great way to play with Kubernetes!
Also check out the [developer documentation](docs/README.md).

## Installation

Planet images are distributed to remote sites via [Gravitational Orbit](https://github.com/gravitational/orbit/blob/master/README.md).

Orbit is a package manager that helps distribute arbitrary files, with versioning, 
across many Linux clusters (like AWS accounts). Planet tarball already contains an Orbit manifest, 
which makes it an Orbit package.

Before proceeding with orbit for the first time, run orbit to generate a credentials file:
```
orbit login
```

It is also recommended to use a local package directory (unless running as root):
```
mkdir -p /tmp/orbit && orbit -p /tmp/orbit login 
```
will use /tmp/orbit as a package directory.


We have an `Orbit` repository running on AWS. It is actually the easiest (and recommended) way to 
install Planet. To see which builds/versions are available and to get the latest run:

```bash
orbit list-remote planet-dev
orbit pull-latest planet-dev
```

See? You now have the latest version and it happens to be `0.0.35`.

```
> orbit list
* planet-dev:0.0.35
```

## Start Planet

Planet package is a pre-packaged Kubernetes. And needs a site-specific configuration to run. `Orbit` allows you to 
specify configuration as key-value pairs and store it as another, _site-local_ package. This enables upgrading  
upgrades independently of their configuration (again, because configuration key/values are stored in another package).

This means that `Planet` needs two packages to run: the main package and the configuration package, which 
you need to create and store locally before running.

To create a configuratin package and store it locally as `planet-cfg:0.0.1`:

```bash
orbit configure planet-dev:0.0.35 \
    planet-cfg:0.0.1 -- \
    --role=master --role=node\
    --volume=/var/planet/etcd:/ext/etcd\
    --volume=/var/planet/registry:/ext/registry\
    --volume=/var/planet/docker:/ext/docker
```

Now you can start `Planet` with the generated configuration (it needs `sudo`):

```bash
sudo orbit exec-config start planet-dev:0.0.35 start planet-cfg:0.0.1
```

## Other Commands

Planet is a generic `container image`. It is basically tarballed and gzipped rootfs.
Usually these images are distributed and updated by [Orbit](https://github.com/gravitational/orbit).

Inside a container image there are Kubernetes components and the Planet binary in `rootfs/usr/bin/planet`.
When you launch that binary, it will self-containerize within its RootFS and will launch all Kubernetes
components using systemd.

If you start it without any commands, it will show the usage info:

```
Commands:
  help   [<command>...]               Show help.
  start  [<flags>] [<rootfs>]         Start orbit container
  stop   [<rootfs>]                   Stop the container
  enter  [<flags>] [<rootfs>] [<cmd>] Enter running the container
  status [<rootfs>]                   Get status of a running container
```

Here's how to stop Planet, for example:

```bash
orbit exec-config stop planet/dev:0.0.1 planet/cfg:0.0.1
```

## Hacking on Planet

The section below is for developers who want to make changes to `Planet`. Orbit is not needed in this case,
you will be running planet directly.

### Building (installing from source)

You must have `Docker >= 1.8.2` installed (and its daemon running) to build Planet. This means you
should be in `docker` group and being able to run typical Docker commands like `docker run` without 
using `sudo`.

Also, if using [Vagrant](https://www.vagrantup.com/downloads.html), make sure you have Vagrant 
version `1.7.4` or newer. This building process has been tested on `Debian 8` and `Ubuntu 15.04`.

The output of Planet build is a tarball that goes into `build/$TARGET`.
There are four targets:

* `make build` - go installs a planet binary and also copies it into $BUILDDIR/current
* `make master` - builds an image containing only Kubernetes master components (kube-api, etcd, etc)
* `make node` - builds an image containing only Kubernets node components (kubelet, kube-proxy)
* `make dev` - builds a combined (master+node) image. _This is what you will be hacking on_.

These take a while to build at first, but subsequent builds are much faster because intermediate 
results are cached. To clear and rebuild from scratch, run one of the following 
(depending which target you want to wipe out): `make dev-clean`, `make node-clean` or `make master-clean`

If you want to clear everything, simply run `make clean`.

After building a combined image (`make dev`) - complement it via `make build` to install / copy the planet binary.

### Starting "Dev" image

Run: 

```
make dev-start
```

You will need another terminal to interact with it. To enter into a running Planet container, 
you'll need to execute `make enter`. 

You will see Kubernetes components running, with `ps -e` showing something like:

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

### Making changes

If you have completed the steps above, you can quickly iterate (make code changes + running results) 
by repeating the following:

0. `make dev` to have an image with everything.
1. Change code.
2. Simply run `make` to quickly compile changes and send the results into the existing image
2. `make dev-start` to run
3. `make stop` to stop
4. Go to step `1.`
 
### Production Mode

To start Planet on a real cloud in production mode you'll have to start Kubernetes-master and Kubernetes-node instances
separately. Here's how you do this for AWS (add more providers in the future).

First, create more than two AWS instances. You'll need one istance for Kubernetes master image, and one for each 
of Kubernets nodes.

Upload `$BUILD/planet-master.tar.gz` to the master AWS instance, untar it using `tar -xzf` and find `planet` executable
inside of `rootfs` directory of the image. Then you can start Planet and it will containerize itself:

```
./rootfs/usr/bin/planet --role=master --cloud-provider=aws \
        --env AWS_ACCESS_KEY_ID:<key-id> \
        --env AWS_SECRET_ACCESS_KEY:<key>
```

Similarly, upload & untar the planet-node image onto each AWS node instance and run:

```
.rootfs/usr/bin/planet --role=node \
    --master-ip=<master-ip> \
    --cloud-provider=aws \
    --env AWS_ACCESS_KEY_ID:<key-id> \
    --env AWS_SECRET_ACCESS_KEY:<key>
```

### Publishing your own Planet build

If you're hacking on Planet and have a new build, this is how to add it (as an Orbit package) into your local Orbit repository:

```bash
orbit import planet-dev.tar.gz planet/dev:0.0.1
```
Now when you have Planet in your local Orbit repo, you can push it to remote Orbit repository by running `orbit push`,
and install it onto any site with `orbit pull` as shown above.

`Planet` is no different than any other Orbit package. Consult [Orbit documentation](https://github.com/gravitational/orbit/blob/master/README.md) to learn more.
