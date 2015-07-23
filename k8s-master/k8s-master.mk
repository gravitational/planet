.PHONY: all

VER := 1.0.1
OS := linux
ARCH := amd64
URL := https://storage.googleapis.com/kubernetes-release/release

BINARIES := $(BUILDDIR)/kube-apiserver $(BUILDDIR)/kube-controller-manager $(BUILDDIR)/kube-scheduler

all: k8s-master.mk $(BINARIES)
	cp -af ./generate-serviceaccount-key.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/generate-serviceaccount-key.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/

	cp -af ./kube-apiserver.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/kube-apiserver.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/

	cp -af ./kube-controller-manager.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/kube-controller-manager.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/

	cp -af ./kube-scheduler.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/kube-scheduler.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/

	install -m 0755 $(BUILDDIR)/kube-apiserver $(ROOTFS)/usr/bin
	install -m 0755 $(BUILDDIR)/kube-controller-manager $(ROOTFS)/usr/bin
	install -m 0755 $(BUILDDIR)/kube-scheduler $(ROOTFS)/usr/bin

$(BINARIES):
	curl -L -o $(BUILDDIR)/$(notdir $@) $(URL)/v$(VER)/bin/linux/$(ARCH)/$(notdir $@)
