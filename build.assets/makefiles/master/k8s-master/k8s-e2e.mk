.PHONY: all

all: k8s-e2e.mk
	@echo "\n---> Placing e2e binaries to rootfs"
	install -m 0755 $(ASSETDIR)/k8s-$(KUBE_VER)/e2e.test $(ROOTFS)/usr/bin
	install -m 0755 $(ASSETDIR)/k8s-$(KUBE_VER)/ginkgo $(ROOTFS)/usr/bin
