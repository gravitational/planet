# Planet Design Doc

#### Intro

Planet is a small Golang executable. It is distributed as a part of a container image, i.e. it 
expects to reside in `rootfs/usr/bin/planet`

When Planet starts, it:

* Uses [libcontainer](https://github.com/docker/libcontainer) to self-containerize 
* Launches [systemd](http://0pointer.de/blog/projects/systemd-docs.html)
* `systemd` instantiates Kubernetes services
* `systemd` also instantiates any other services we decide to run alongside Kubernetes

#### Building

To build `planet` you need Docker > 1.6.2. The build process is split into 4 stages:

1. Download and prepare a base OS image. This is done via `make os-image` and it uses Ubuntu 15.04. Everything will be built inside of (and based upon) it. The output of this step is `planet/os` Docker image.
2. Build the "base", this is done via `make planet-base` target. It adds docker registry, docker itself and Flannel to the base image and saves the output as `planet/base` docker image.
3. Copy the source of Planet into `planet/base` image and buildng it inside of it.
4. Download and install Kubernetes inside of a Docker image, then prepare our own [Orbit image](https://github.com/gravitational/orbit) using dependencies from the base image and. The output is an `aci` file.

