.PHONY: all

all: k8s-node.mk
	cp -af ./kube-kubelet.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/kube-kubelet.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/

	cp -af ./kube-proxy.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/kube-proxy.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/

	install -m 0755 $(BUILDDIR)/kube-proxy $(ROOTFS)/usr/bin
	install -m 0755 $(BUILDDIR)/kubelet $(ROOTFS)/usr/bin
