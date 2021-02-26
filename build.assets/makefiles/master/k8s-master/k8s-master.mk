.PHONY: all

BINDIR:=$(ASSETDIR)/k8s-$(KUBE_VER)

all: k8s-master.mk scripts
	@echo "\n---> Building Kubernetes master components\n"
	mkdir -p $(ROOTFS)/etc/kubernetes
	cp -TRv -p rootfs/etc/kubernetes $(ROOTFS)/etc/kubernetes
	cp -af ./kube-apiserver.service $(ROOTFS)/lib/systemd/system
	cp -af ./kube-controller-manager.service $(ROOTFS)/lib/systemd/system
	cp -af ./kube-scheduler.service $(ROOTFS)/lib/systemd/system
	cp -af ./kube-kubelet.service $(ROOTFS)/lib/systemd/system
	cp -af ./kube-proxy.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/kube-kubelet.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/
	ln -sf /lib/systemd/system/kube-proxy.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/
	install -m 0755 $(BINDIR)/kube-apiserver $(ROOTFS)/usr/bin
	install -m 0755 $(BINDIR)/kube-controller-manager $(ROOTFS)/usr/bin
	install -m 0755 $(BINDIR)/kube-scheduler $(ROOTFS)/usr/bin
	install -m 0755 $(BINDIR)/kubectl $(ROOTFS)/usr/bin
	install -m 0755 $(BINDIR)/kube-proxy $(ROOTFS)/usr/bin
	install -m 0755 $(BINDIR)/kubelet $(ROOTFS)/usr/bin

.PHONY: scripts
scripts:
# install script to update clusterDNS values in kubelet config.
	mkdir -p $(ROOTFS)/usr/bin/scripts
	install -m 0755 ./cluster-dns.sh $(ROOTFS)/usr/bin/scripts/cluster-dns.sh