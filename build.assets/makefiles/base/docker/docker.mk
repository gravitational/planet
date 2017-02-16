# This makefile installs Kubernetes-friendly docker inside of planet
# images. 
# 
# This makefile is executed inside the docker's buildbox image.

REGISTRY := apiserver:5000

.PHONY: all
all: service scripts certs

.PHONY: service
service:
# install docker daemon service
	@echo "\n---> Installing Docker:\n"
	cp -af ./docker.service $(ROOTFS)/lib/systemd/system/docker.service
	ln -sf /lib/systemd/system/docker.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/

.PHONY: scripts
scripts:
# install the unmount cleanup script
	mkdir -p $(ROOTFS)/usr/bin/scripts
	install -m 0755 ./unmount-devmapper.sh $(ROOTFS)/usr/bin/scripts/unmount-devmapper.sh

.PHONY: certs
certs:
# client and server certificate directory
	mkdir -p $(ROOTFS)/etc/docker/certs.d/$(REGISTRY)
# notice .crt for roots, and .cert for certificates, this is not a typo, but docker expected format
	ln -sf /var/state/root.cert $(ROOTFS)/etc/docker/certs.d/$(REGISTRY)/$(REGISTRY).crt
	ln -sf /var/state/kubelet.cert $(ROOTFS)/etc/docker/certs.d/$(REGISTRY)/client.cert
	ln -sf /var/state/kubelet.key $(ROOTFS)/etc/docker/certs.d/$(REGISTRY)/client.key
