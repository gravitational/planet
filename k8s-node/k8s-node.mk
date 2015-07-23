.PHONY: all

VER := 1.0.1
OS := linux
ARCH := amd64
URL := https://storage.googleapis.com/kubernetes-release/release

BINARIES := $(BUILDDIR)/kube-proxy $(BUILDDIR)/kubelet

all: k8s-node.mk $(BINARIES)
	cp -af ./kube-kubelet.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/kube-kubelet.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/

	cp -af ./kube-proxy.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/kube-proxy.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/

	install -m 0755 $(BUILDDIR)/kube-proxy $(ROOTFS)/usr/bin
	install -m 0755 $(BUILDDIR)/kubelet $(ROOTFS)/usr/bin

$(BINARIES):
	curl -L -o $(BUILDDIR)/$(notdir $@) $(URL)/v$(VER)/bin/linux/$(ARCH)/$(notdir $@)
