# This makefile installs Kubernetes-friendly docker inside of planet
# images.
#
# This makefile is executed inside the docker's buildbox image.

REGISTRY_ALIASES := apiserver:5000 \
		leader.telekube.local:5000 \
		leader.gravity.local:5000 \
		registry.local:5000

.PHONY: all
all: service scripts certs

.PHONY: service
service:
# install docker daemon service
	@echo "\n---> Installing Docker:\n"
	cp -af ./docker.service $(ROOTFS)/lib/systemd/system/docker.service
	cp -af ./docker.socket $(ROOTFS)/lib/systemd/system/docker.socket
	ln -sf /lib/systemd/system/docker.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/

.PHONY: scripts
scripts:
# install the unmount cleanup script
	mkdir -p $(ROOTFS)/usr/bin/scripts
	install -m 0755 ./unmount-devmapper.sh $(ROOTFS)/usr/bin/scripts/unmount-devmapper.sh

.PHONY: certs
certs:
# create client and server certificate directories
# notice .crt for roots, and .cert for certificates, this is not a typo, but docker expected format
	for r in $(REGISTRY_ALIASES); do \
		mkdir -p $(ROOTFS)/etc/docker/certs.d/$$r; \
		ln -sf /var/state/root.cert $(ROOTFS)/etc/docker/certs.d/$$r/$$r.crt; \
		ln -sf /var/state/kubelet.cert $(ROOTFS)/etc/docker/certs.d/$$r/client.cert; \
		ln -sf /var/state/kubelet.key $(ROOTFS)/etc/docker/certs.d/$$r/client.key; \
	done
