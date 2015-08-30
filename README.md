# Cube

Installable Kubernetes delivered in Orbit containers

Installation
-------------

**IMPORTANT** the build process relies on Docker > 1.6.2. Install docker to make sure your build succeeds.


If this is your first time building the project:

```
make cube-os
```

Then:

```
make
```

Running in dev mode:
```
make dev
make start-dev
```

Entering cube's namespace:

```
make enter
```

AWS
---
Master:

```
kube-master/cube --cloud-provider=aws --env AWS_ACCESS_KEY_ID=AKIAJY6HPQAX6CJJUAHQ --env AWS_SECRET_ACCESS_KEY=<key>  kube-master/rootfs/

```

Node:

```
kube-node/cube --master-ip=172.31.15.90 --cloud-provider=aws --env AWS_ACCESS_KEY_ID=AKIAJY6HPQAX6CJJUAHQ --env AWS_SECRET_ACCESS_KEY=<key>  kube-node/rootfs/
```




Description
-------------

Initial goal of the project:

1. Provide orbit images for:

* Kubernetes master (etcd2, docker, flanneld, api-service, scheduler)
* Kubernetes node (docker, flanneld, kube-proxy, kube-node)

2. Give clear way to "deploy" real application into our embedded cluster

let's use wordpress as a test example: https://github.com/GoogleCloudPlatform/kubernetes/tree/v1.0.1/examples/mysql-wordpress-pd
It has enough moving parts.

3. Write an installer that installs Cube-powered Kubernetes cluster to various environments:

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



