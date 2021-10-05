.PHONY: all

BIN := $(ASSETDIR)/aws-encryption-provider

all: $(BIN) aws-encryption-provider.service
	@echo "\n---> Installing aws-encryption-provider:\n"

	cp -af $(BIN) $(ROOTFS)/usr/bin/aws-encryption-provider
	cp -af ./aws-encryption-provider.service $(ROOTFS)/lib/systemd/system
	mkdir -p $(ROOTFS)/etc/kmsplugin
# These permissions allow the encryption-provider.sock socket file to be created with planet ownership.
	chmod o+t $(ROOTFS)/etc/kmsplugin
	chmod a+rwx $(ROOTFS)/etc/kmsplugin

$(BIN):
	mkdir /tmp/kubernetes-sigs
	cd /tmp/kubernetes-sigs
	git clone https://github.com/kubernetes-sigs/aws-encryption-provider
	cd /tmp/kubernetes-sigs/aws-encryption-provider
	git checkout $(AWS_ENCRYPTION_PROVIDER_VER)
	make build-server
	cp /tmp/kubernetes-sigs/aws-encryption-provider/bin/aws-encryption-provider $@
	rm -rf /tmp/kubernetes-sigs