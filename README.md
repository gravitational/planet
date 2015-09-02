# Planet

Planet is a containerized Kubernetes environment, it is a self-containerizing Ubuntu 15.04 image with
Kubernetes services running inside. 

There are [official ways](http://kubernetes.io/v1.0/docs/getting-started-guides/README.html) to install and 
play with Kubernetes, but `Planet` differs from those because:

* Planet creates a "bubble of consistency" for every Kubernetes cluster we deploy.
* Planet allows to packge our own services running under/alongside Kubernetes.
* Planet facilitates easier remote updating of itself and for Kubernetes (because it uses [Orbit containers](https://github.com/gravitational/orbit))

It also happens to be a great way to play with Kubernetes!
Also check out the [developer documentation](docs/README.md).

Installation
-------------

**IMPORTANT** the build process relies on Docker > 1.6.2. When installing Docker on Virtualbox/vagrant you may 
end up with a VM which doesn't boot (hangs during shared volume mounting). Do `apt-get dist-upgrade` to fix that.

If this is your first time building the project, create a docker OS image (slow):

```
make os-image
```

Then you can build Planet itself. There are two builds: "development" and "production".
By default `make` with no argumnets builds the production image. Here's how to build both:

```
make
make dev
```

**NOTE** the output of make command is usually a container image. For example `make dev` 
creates `$HOME/build/planet-dev.aci`

Development Mode
----------------
Planet can run in "development mode" when a single container contains both 
Kubernetes master and Kubernetes node. This allows to launch a fully functional 
single-node "cluster" on your laptop:

```
make start-dev
```

You'll see how various services are starting and then it will stop. To play with this tiny "cluster" you'll need
to enter it (use another terminal session):

```
make enter
```

You're inside of a container now which runs all Kubernetes components, run `ps -e` and you'll see something like:

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

Production Mode
---------------

To start Planet on a real cloud in production mode you'll have to start Kubernetes-master and Kubernetes-node instances
separately. Here's how you do this for AWS (add more providers in the future).

First, create more than two AWS instances. You'll need one istance for Kubernetes master image, and one for each 
of Kubernets nodes.

Upload `$BUILD/planet-master.tar.gz` to the master AWS instance, untar it using `tar -xzf` and find `planet` executable
inside of `rootfs` directory of the image. Then you can start Planet and it will containerize itself:

```
./rootfs/usr/bin/planet --cloud-provider=aws --env AWS_ACCESS_KEY_ID=AKIAJY6HPQAX6CJJUAHQ --env AWS_SECRET_ACCESS_KEY=<key>  kube-master/rootfs/

```

Similarly, upload & untar the planet-node image onto each AWS node instance and run:

```
.rootfs/usr/bin/planet --master-ip=172.31.15.90 --cloud-provider=aws --env AWS_ACCESS_KEY_ID=AKIAJY6HPQAX6CJJUAHQ --env AWS_SECRET_ACCESS_KEY=<key>  kube-node/rootfs/
```

Planet and Orbit
----------------

Orbit is a package manager that helps to distribute various blobs across the infrastructure. Planet tarball already contains Orbit manifest, so no
extra actions are necessary.

Example of using orbit and planet (assuming orbit is installed)

**Import planet tarball**

```bash
orbit import planet-dev.tar.gz planet/dev#0.0.1
```

**Configure planet package***

Configure local planet package by using `orbit configure` command. It will capture and validate the arguments and
will generate a special configuration package `planet/dev-cfg#0.0.1`

```bash
orbit configure planet/dev#0.0.1 \
    planet/dev-cfg#0.0.1 args\
    --role=master --role=node\
    --volume=/var/planet/etcd:/ext/etcd\
    --volume=/var/planet/registry:/ext/registry\
    --volume=/var/planet/docker:/ext/docker\
    --volume=/var/planet/mysql:/ext/mysql
```

**Start planet**

This command will execute `start` command supplied by `planet/dev#0.0.1` and will use configuration from `planet/dev-cfg#0.0.1` that
we've just generated

```bash
orbit exec-config start planet/dev#0.0.1 planet/cfg#0.0.1
```

**Other commands**

Execute enter, status and stop commands using the same pattern as above:

```bash
orbit exec-config stop planet/dev#0.0.1 planet/cfg#0.0.1
```


Using Planet
------------

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
