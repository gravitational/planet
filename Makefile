SHELL:=/bin/bash
.PHONY: all dev planet planet-os planet-base planet-master planet-node planet-dev kill-systemd enter clean start-dev stop status remove-godeps remove-temp-files

BUILDDIR := $(HOME)/build
export

all: planet-os planet-master planet-node planet

dev: planet-dev
	cd $(BUILDDIR) && tar -xzf planet-dev.tar.gz

# Builds 'planet' binary
planet: remove-temp-files
	go build -o $(BUILDDIR)/planet github.com/gravitational/planet/tool/planet
	go build -o /var/orbit/unpack/planet-dev/0.0.17/bd9fb/rootfs/usr/bin/planet github.com/gravitational/planet/tool/planet

remove-temp-files:
	find . -name flymake_* -delete

test-package: remove-temp-files
	go test -v ./$(p)

# Builds systemd-based "distro" using Ubuntu 15.04 This distro is used as a base OS image
# for building and running Kubernetes.
planet-os:
	sudo docker build --no-cache=true -t planet/os -f makefiles/os/os.dockerfile . ;\

# This target builds on top of os-image step above. It builds a new docker image, using planet/os
# and adds docker registry, docker and flannel
planet-base:
	sudo docker build --no-cache=true -t planet/base -f makefiles/base/base.dockerfile .

# Uses planet/base docker image as a foundation, it downloads and installs Kubernetes
# master components: API, etcd, etc.
planet-master: planet-base
	sudo docker build -t planet/master -f makefiles/master/master.dockerfile .
	mkdir -p $(BUILDDIR)
	rm -rf $(BUILDDIR)/planet-master.tar.gz
	id=$$(sudo docker create planet/master:latest) && sudo docker cp $$id:/build/planet-master.tar.gz $(BUILDDIR)

# Uses planet/base docker image as a foundation, it downloads and installs Kubernetes
# node components: kubelet and kube-proxy
planet-node: planet-base
	sudo docker build -t planet/node -f makefiles/node/node.dockerfile .
	mkdir -p $(BUILDDIR)
	rm -f $(BUILDDIR)/planet-node.tar.gz
	id=$$(sudo docker create planet/node:latest) && sudo docker cp $$id:/build/planet-node.tar.gz $(BUILDDIR)/

# Uses planet/base docker image as a foundation, combines 'master' and 'node' into a single image
planet-dev: planet-base
	sudo docker build -t planet/dev -f makefiles/dev/dev.dockerfile .
	mkdir -p $(BUILDDIR)
	rm -f $(BUILDDIR)/planet-dev.tar.gz
	id=$$(sudo docker create planet/dev:latest) && sudo docker cp $$id:/build/planet-dev.tar.gz $(BUILDDIR)

# In case if something goes wrong, use this program to kill systemd that runs the whole setup
kill-systemd:
	sudo kill -9 $$(ps uax  | grep [/]bin/systemd | awk '{ print $$2 }')


# IMPORTANT NOTES for installer
# * We need to set cloud provider for kubernetes - semi done, aws
# * Flanneld needs NET_ADMIN and modpropbe, so we need to mount /lib/modules - done
# * Kube-node needs master private IP - done

# Have a unified way to generate environment for master and node in a consistent way and use one file everywhere -done
# what's the problem with udevd (turn it off probably) ?
# cgroups should be mounted in systemd compatible way (cpu,cpuacct)

# kernel version on ubuntu 14.04, docker with overlayfs needs new kernel. Devicemapper is not stable.
# sudo apt-get install linux-headers-generic-lts-vivid linux-image-generic-lts-vivid
# check kernel version, and if it's less than 3.18 back off

enter:
	cd $(BUILDDIR) && sudo $(BUILDDIR)/rootfs/usr/bin/planet enter --debug

clean:
	rm -rf $(BUILDDIR)/master/rootfs/run/planet.socket

start-dev:
	@sudo mkdir -p /var/planet/registry /var/planet/etcd /var/planet/docker /var/planet/mysql
	cd $(BUILDDIR) && sudo $(BUILDDIR)/rootfs/usr/bin/planet start\
		--role=master\
		--role=node\
		--volume=/var/planet/etcd:/ext/etcd\
		--volume=/var/planet/registry:/ext/registry\
		--volume=/var/planet/docker:/ext/docker\
        --volume=/var/planet/mysql:/ext/mysql

stop:
	cd $(BUILDDIR) && sudo $(BUILDDIR)/rootfs/usr/bin/planet stop

status:
	sudo $(BUILDDIR)/rootfs/usr/bin/planet status $(BUILDDIR)/rootfs

remove-godeps:
	rm -rf Godeps/
	find . -iregex .*go | xargs sed -i 's:".*Godeps/_workspace/src/:":g'
