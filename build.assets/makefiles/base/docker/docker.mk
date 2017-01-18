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
	$(DIR)/docker \
	$(DIR)/docker-containerd \
	$(DIR)/docker-containerd-ctr \
	$(DIR)/docker-containerd-shim \
	$(DIR)/dockerd \
	$(DIR)/docker-proxy \
	$(DIR)/docker-runc
TARBALL := $(DIR)/docker.tgz

REGISTRY := apiserver:5000

$(ROOTFS)/usr/bin/docker: $(BINARIES)
# install socket-activated metadata service
	@echo "\n---> Installing Docker to be used with Kubernetes:\n"
	cp -af ./docker.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/docker.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/
	cp -af ./docker.socket $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/docker.socket $(ROOTFS)/lib/systemd/system/sockets.target.wants/

# install the unmount cleanup script
	mkdir -p $(ROOTFS)/usr/bin/scripts
	install -m 0755 ./unmount-devmapper.sh $(ROOTFS)/usr/bin/scripts

# copy docker from the build dir into rootfs:
	mkdir -p $(ROOTFS)/usr/bin
	cp $(BINARIES) $(ROOTFS)/usr/bin/docker

# that's a directory with client and server certs
	mkdir -p $(ROOTFS)/etc/docker/certs.d/$(REGISTRY)
# notice .crt for roots, and .cert for certificates, this is not a typo, but docker expected format
	ln -sf /var/state/root.cert $(ROOTFS)/etc/docker/certs.d/$(REGISTRY)/$(REGISTRY).crt
	ln -sf /var/state/kubelet.cert $(ROOTFS)/etc/docker/certs.d/$(REGISTRY)/client.cert
	ln -sf /var/state/kubelet.key $(ROOTFS)/etc/docker/certs.d/$(REGISTRY)/client.key


$(BINARIES):
	tar --strip-components=1 -xvzf $(TARBALL) -C $(DIR)
	rm $(TARBALL)

$(TARBALL):
# download release
	mkdir -p $(DIR)
	curl -L https://get.docker.com/builds/$(OS)/$(ARCH)/docker-$(VER).tgz -o $@
