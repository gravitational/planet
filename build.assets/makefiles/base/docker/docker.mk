# This makefile installs Kubernetes-friendly docker inside of planet
# images. 
# 
# This makefile itself is executed inside the docker's buildbox image.
.PHONY: all

ARCH := x86_64
OS := Linux
VER := 1.12.6
override DIR := $(ASSETDIR)/docker-$(VER)
BINARIES := \
	docker \
	docker-containerd \
	docker-containerd-ctr \
	docker-containerd-shim \
	dockerd \
	docker-proxy \
	docker-runc
ASSET_BINARIES := $(addprefix $(DIR)/, $(BINARIES))
ROOTFS_BINARIES := $(addprefix $(ROOTFS)/usr/bin/, $(BINARIES))
TARBALL := $(ASSETDIR)/docker-$(VER).tgz

REGISTRY := apiserver:5000

$(ROOTFS_BINARIES): $(BINARIES)
# install socket-activated metadata service
	@echo "\n---> Installing Docker:\n"
	cp -af ./docker.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/docker.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/
	cp -af ./docker.socket $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/docker.socket $(ROOTFS)/lib/systemd/system/sockets.target.wants/

# install the unmount cleanup script
	mkdir -p $(ROOTFS)/usr/bin/scripts
	install -m 0755 ./unmount-devmapper.sh $(ROOTFS)/usr/bin/scripts

# copy docker from the build dir into rootfs:
	mkdir -p $(ROOTFS)/usr/bin
	cp $(DIR)/* $(ROOTFS)/usr/bin/

# that's a directory with client and server certs
	mkdir -p $(ROOTFS)/etc/docker/certs.d/$(REGISTRY)
# notice .crt for roots, and .cert for certificates, this is not a typo, but docker expected format
	ln -sf /var/state/root.cert $(ROOTFS)/etc/docker/certs.d/$(REGISTRY)/$(REGISTRY).crt
	ln -sf /var/state/kubelet.cert $(ROOTFS)/etc/docker/certs.d/$(REGISTRY)/client.cert
	ln -sf /var/state/kubelet.key $(ROOTFS)/etc/docker/certs.d/$(REGISTRY)/client.key


$(BINARIES): $(TARBALL)
	tar --strip-components=1 -xvzf $(TARBALL) -C $(DIR)/

$(TARBALL):
# download release
	@echo "\n---> Downloading Docker:\n"
	mkdir -p $(DIR)
	curl -L https://get.docker.com/builds/$(OS)/$(ARCH)/docker-$(VER).tgz -o $@
