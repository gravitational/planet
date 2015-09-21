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
		echo "planet/os already exists. Run docker rmi planet/os to rebuild" ;\
	fi

base: os
	@if [[ ! $$(docker images | grep planet/base) ]]; then \
		cd build/docker; docker build --no-cache=true -t planet/base -f base.dockerfile . ;\
	else \
		echo "planet/base already exists. Run 'docker rmi planet/base' to rebuild" ;\
	fi

# Makes a docker image (build box) which is used to build everything else. It's based
# on the 'os' base + developer tools.
buildbox: base
	@if [[ ! $$(docker images | grep planet/buildbox) ]]; then \
		cd build/docker; docker build --no-cache=true -t planet/buildbox -f buildbox.dockerfile . ;\
	else \
		echo "planet/bulidbox already exists. Run 'docker rmi planet/buildbox' to rebuild" ;\
	fi

# Makes a "developer" image, with _all_ parts of Kubernetes installed
dev: buildbox rootfs $(ROOTFS)/usr/bin/planet
	docker run -ti --rm=true \
		--volume=$$(pwd)/build:/build \
		--env="ROOTFS=/$(ROOTFS)" \
		--env="OUT=/$(OUT)" \
		planet/buildbox \
		/bin/bash /build/scripts/dev.sh
	@cp $(BUILDDIR)/makefiles/orbit.manifest.json $(BUILDDIR)/
	cd $(BUILDDIR) && tar -czf dev.tar.gz orbit.manifest.json rootfs

# Makes a "master" image, with only master components of Kubernetes installed
master: buildbox rootfs $(ROOTFS)/usr/bin/planet
	docker run -ti --rm=true \
		--volume=$$(pwd)/build:/build \
		--env="ROOTFS=/$(ROOTFS)" \
		--env="OUT=/$(OUT)" \
		planet/buildbox \
		/bin/bash /build/scripts/master.sh
	@cp $(BUILDDIR)/makefiles/orbit.manifest.json $(BUILDDIR)/
	cd $(BUILDDIR) && tar -czf master.tar.gz orbit.manifest.json rootfs

# Makes a "node" image, with only node components of Kubernetes installed
node: buildbox rootfs $(ROOTFS)/usr/bin/planet
	docker run -ti --rm=true \
		--volume=$$(pwd)/build:/build \
		--env="ROOTFS=/$(ROOTFS)" \
		--env="OUT=/$(OUT)" \
		planet/buildbox \
		/bin/bash /build/scripts/node.sh
	cp $(BUILDDIR)/makefiles/orbit.manifest.json $(BUILDDIR)/
	cd $(BUILDDIR) && tar -czf node.tar.gz orbit.manifest.json rootfs

# builds planet binary and installs it into rootfs
$(ROOTFS)/usr/bin/planet: build

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


test-package: remove-temp-files
	go test -v ./$(p)


enter:
	cd $(BUILDDIR) && sudo rootfs/usr/bin/planet enter --debug /bin/bash

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
