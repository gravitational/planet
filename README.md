# Cube

Installable Kubernetes delivered in Orbit containers

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



