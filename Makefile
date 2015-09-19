SHELL:=/bin/bash
.PHONY: all dev planet planet-os planet-base planet-master planet-node planet-dev kill-systemd enter clean start stop status remove-godeps remove-temp-files

BUILDDIR := $(HOME)/planet.build
export

# Builds 'planet' binary
planet: check-rootfs
	go install github.com/gravitational/planet/tool/planet
	@ln -sf $$GOPATH/bin/planet $(BUILDDIR)/rootfs/usr/bin/planet

remove-temp-files:
	@mkdir -p $(BUILDDIR)
	find . -name flymake_* -delete

test-package: remove-temp-files
	go test -v ./$(p)

# Builds systemd-based "distro" using Ubuntu 15.04 This distro is used as a base OS image
# for building and running Kubernetes.
planet-os:
	@if [[ ! $$(docker images | grep planet/os) ]]; then \
		docker build --no-cache=true -t planet/os -f makefiles/os/os.dockerfile . ;\
	fi

# This target builds on top of os-image step above. It builds a new docker image, using planet/os
# and adds docker registry, docker and flannel
planet-base: planet-os remove-temp-files
	@if [[ ! $$(docker images | grep planet/base) ]]; then \
		docker build --no-cache=true -t planet/base -f makefiles/base/base.dockerfile . ; \
	fi
	mkdir -p $(BUILDDIR)

# Uses planet/base docker image as a foundation, it downloads and installs Kubernetes
# master components: API, etcd, etc.
planet-master: planet-base
	sudo rm -rf $(BUILDDIR)/rootfs
	docker build -t planet/master -f makefiles/master/master.dockerfile . ; \
	rm -rf $(BUILDDIR)/planet-master.tar.gz
	id=$$(docker create planet/master:latest) && docker cp $$id:/build/planet-master.tar.gz $(BUILDDIR)
	cd $(BUILDDIR) && tar -xzf planet-master.tar.gz

# Uses planet/base docker image as a foundation, it downloads and installs Kubernetes
# node components: kubelet and kube-proxy
planet-node: planet-base
	sudo rm -rf $(BUILDDIR)/rootfs
	docker build -t planet/node -f makefiles/node/node.dockerfile . ; \
	rm -f $(BUILDDIR)/planet-node.tar.gz
	id=$$(docker create planet/node:latest) && docker cp $$id:/build/planet-node.tar.gz $(BUILDDIR)/
	cd $(BUILDDIR) && tar -xzf planet-node.tar.gz

# Uses planet/base docker image as a foundation, combines 'master' and 'node' into a single image
planet-dev: planet-base
	sudo rm -rf $(BUILDDIR)/rootfs
	docker build -t planet/dev -f makefiles/dev/dev.dockerfile .
	rm -f $(BUILDDIR)/planet-dev.tar.gz
	id=$$(docker create planet/dev:latest) && docker cp $$id:/build/planet-dev.tar.gz $(BUILDDIR)
	cd $(BUILDDIR) && tar -xzf planet-dev.tar.gz

clean:
	@bash makefiles/remove-docker-image planet/os planet/base planet/node planet/master planet/dev
	rm -rf $(BUILDDIR)

check-rootfs:
	@if [ ! -d $(BUILDDIR)/rootfs/bin ] ; then \
		echo -e "\nDid you select a build first?\nRun 'make planet-dev' or 'make node' or 'make master' before running 'make'\n" ;\
		exit 1 ; \
	fi

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

start:
	@sudo mkdir -p /var/planet/registry /var/planet/etcd /var/planet/docker 
	@sudo chown $$USER:$$USER /var/planet/etcd -R
	cd $(BUILDDIR) && sudo $(BUILDDIR)/rootfs/usr/bin/planet --debug start\
		--role=master\
		--role=node\
		--volume=/var/planet/etcd:/ext/etcd\
		--volume=/var/planet/registry:/ext/registry\
		--volume=/var/planet/docker:/ext/docker

stop:
	cd $(BUILDDIR) && sudo $(BUILDDIR)/rootfs/usr/bin/planet --debug stop

status:
	sudo $(BUILDDIR)/rootfs/usr/bin/planet status $(BUILDDIR)/rootfs

remove-godeps:
	rm -rf Godeps/
	find . -iregex .*go | xargs sed -i 's:".*Godeps/_workspace/src/:":g'

all: planet-os planet-master planet-node planet
