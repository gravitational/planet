.PHONY: all

all: k8s-e2e.mk
	@echo "\n---> Placing e2e binaries to rootfs"
	install -m 0755 $(TARGETDIR)/e2e.test $(ROOTFS)/usr/bin
	install -m 0755 $(TARGETDIR)/ginkgo $(ROOTFS)/usr/bin
