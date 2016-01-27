.PHONY: all

BINDIR := $(ASSETDIR)/k8s-$(KUBE_VER)

all: k8s-e2e.mk
	@echo "\n---> Placing e2e binaries to rootfs"
	install -m 0755 $(BINDIR)/e2e.test $(ROOTFS)/usr/bin
	install -m 0755 $(BINDIR)/ginkgo $(ROOTFS)/usr/bin
