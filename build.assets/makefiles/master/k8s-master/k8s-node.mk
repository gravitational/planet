.PHONY: all

BINDIR:=$(ASSETDIR)/k8s-$(KUBE_VER)

all: k8s-node.mk
	@echo "\n---> Building Kubernetes node components\n"
	mkdir -p $(ROOTFS)/etc/kubernetes
	mkdir -p $(ROOTFS)/opt
	cp -TRv -p rootfs/etc/kubernetes $(ROOTFS)/etc/kubernetes
	cp -TRv -p rootfs/opt $(ROOTFS)/opt
	cp -af ./kube-kubelet.service $(ROOTFS)/lib/systemd/system
	cp -af ./kube-proxy.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/kube-kubelet.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/
	ln -sf /lib/systemd/system/kube-proxy.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/
	install -m 0755 $(BINDIR)/kube-proxy $(ROOTFS)/usr/bin
	install -m 0755 $(BINDIR)/kubelet $(ROOTFS)/usr/bin
	install -m 0755 $(BINDIR)/kubectl $(ROOTFS)/usr/bin
