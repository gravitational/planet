SHELL:=/bin/bash
.PHONY: planet build

BUILDDIR := build
ROOTFS := $(BUILDDIR)/rootfs
OUT := $(BUILDDIR)/out
export

build: 
	go install github.com/gravitational/planet/tool/planet
	@ln -sf $$GOPATH/bin/planet $(ROOTFS)/usr/bin/planet

# Builds the base Docker image everything else is based on. Debian stable + configured locales. 
os: 
	@if [[ ! $$(docker images | grep planet/os) ]]; then \
		cd build/docker; docker build --no-cache=true -t planet/os -f os.dockerfile . ;\
	else \
		echo "Not rebuilding planet/os image. Run make clean if you want that" ;\
	fi

base: os
	@if [[ ! $$(docker images | grep planet/base) ]]; then \
		cd build/docker; docker build --no-cache=true -t planet/base -f base.dockerfile . ;\
	else \
		echo "Not rebuilding planet/base image. Run make clean if you want that" ;\
	fi

# Makes a docker image (build box) which is used to build everything else. It's based
# on the 'os' base + developer tools.
buildbox: base
	@if [[ ! $$(docker images | grep planet/buildbox) ]]; then \
		cd build/docker; docker build --no-cache=true -t planet/buildbox -f buildbox.dockerfile . ;\
	else \
		echo "Not rebuilding planet/bulidbox image. Run docker rmi planet/buildbox" ;\
	fi

# Makes a "developer" image, with _all_ parts of Kubernetes installed
dev: buildbox rootfs
	docker run -ti --rm=true \
		--volume=$$(pwd)/build:/build \
		--env="ROOTFS=/$(ROOTFS)" \
		--env="OUT=/$(OUT)" \
		planet/buildbox \
		/bin/bash /build/scripts/dev.sh

# sets up clean rootfs (based on 'os' docker image) in $ROOTFS
rootfs: reset-rootfs
	-docker rm -f planet-base
	docker create --name="planet-base" planet/base
	cd $$ROOTFS && docker export planet-base | tar -x
	docker rm -f planet-base


remove-temp-files:
	find . -name flymake_* -delete
	sudo umount -f $ROOTFS
	sudo rm -rf $ROOTFS


# re-creates the rootfs using ram disk (tmpfs)
reset-rootfs:
	bash build/scripts/reset-rootfs


planet: 
	go install github.com/gravitational/planet/tool/planet
	@ln -sf $$GOPATH/bin/planet $(ROOTFS)/usr/bin/planet

test-package: remove-temp-files
	go test -v ./$(p)


# This target builds on top of os-image step above. It builds a new docker image, using planet/os
# and adds docker registry, docker and flannel
planet-base: planet-os remove-temp-files
	@if [[ ! $$(docker images | grep planet/base) ]]; then \
		docker build --no-cache=true -t planet/base -f makefiles/base/base.dockerfile . ; \
	fi
	mkdir -p $(BUILDDIR)

check-rootfs:
	@if [ ! -d $(BUILDDIR)/rootfs/bin ] ; then \
		echo -e "\nDid you select a build first?\nRun 'make planet-dev' or 'make node' or 'make master' before running 'make'\n" ;\
		exit 1 ; \
	fi

enter:
	cd $(BUILDDIR) && sudo rootfs/usr/bin/planet enter --debug 

start: 
	@sudo mkdir -p /var/planet/registry /var/planet/etcd /var/planet/docker 
	@sudo chown $$USER:$$USER /var/planet/etcd -R
	cd $(BUILDDIR) && sudo rootfs/usr/bin/planet start\
		--role=master\
		--role=node\
		--volume=/var/planet/etcd:/ext/etcd\
		--volume=/var/planet/registry:/ext/registry\
		--volume=/var/planet/docker:/ext/docker

stop:
	cd $(BUILDDIR) && sudo rootfs/usr/bin/planet --debug stop

remove-godeps:
	rm -rf Godeps/
	find . -iregex .*go | xargs sed -i 's:".*Godeps/_workspace/src/:":g'

clean:
	docker rmi planet/os planet/buildbox
