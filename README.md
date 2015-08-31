# Planet

Planet is a Kubernetes installer. It runs Kubernetes inside of a containerized Ubuntu 15.04 image.
There are [official ways](http://kubernetes.io/v1.0/docs/getting-started-guides/README.html) to install and 
play with Kubernetes, but `Planet` differs from those because:

* Planet is built to be self-updating.
* Planet allows us to packge our own services running under/alongside Kubernetes.
* Planet hides the underlying infrastructure/cloud provider from the end user.

In other words, Planet is our "OS" we use to run Kubernetes on top of.
It also happens to be a great way to play with Kubernetes!

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
separately. Here's how you do this for AWS (add more providers in the future):

Master:

```
kube-master/planet --cloud-provider=aws --env AWS_ACCESS_KEY_ID=AKIAJY6HPQAX6CJJUAHQ --env AWS_SECRET_ACCESS_KEY=<key>  kube-master/rootfs/

```

Node:

```
kube-node/planet --master-ip=172.31.15.90 --cloud-provider=aws --env AWS_ACCESS_KEY_ID=AKIAJY6HPQAX6CJJUAHQ --env AWS_SECRET_ACCESS_KEY=<key>  kube-node/rootfs/
```

Description
-----------

Initial goal of the project:

1. Provide orbit images for:

* Kubernetes master (etcd2, docker, flanneld, api-service, scheduler)
* Kubernetes node (docker, flanneld, kube-proxy, kube-node)

2. Give clear way to "deploy" real application into our embedded cluster

let's use wordpress as a test example: https://github.com/GoogleCloudPlatform/kubernetes/tree/v1.0.1/examples/mysql-wordpress-pd
It has enough moving parts.

3. Write an installer that installs Planet-powered Kubernetes cluster to various environments:

* Your laptop
* AWS
* GCE
* Rackspace Bare-Metal

Resume:

At the end of this project we will have installable kubernetes with a real word-press application that can be distributed
to various providers as an installable app.

Notes:

Use this guide to build a custom release of Kubernetes for orbit.
https://github.com/GoogleCloudPlatform/kubernetes/blob/v1.0.1/docs/getting-started-guides/scratch.md
