.PHONY: all

BIN := $(ASSETDIR)/ecr-credential-provider

all: $(BIN)
	@echo "\n---> Installing ecr-credential-provider:\n"
	mkdir -p /opt/ecr-credential-provider/bin /etc/ecr-credential-provider
	cp -af $(BIN) $(ROOTFS)/opt/ecr-credential-provider/bin/ecr-credential-provider

$(BIN):
	mkdir /tmp/kubernetes
	cd /tmp/kubernetes && git clone https://github.com/kubernetes/cloud-provider-aws
	cd /tmp/kubernetes/cloud-provider-aws && git checkout $(AWS_ECR_CREDENTIAL_PROVIDER_VER) && make ecr-credential-provider
	cp /tmp/kubernetes/cloud-provider-aws/bin/ecr-credential-provider $@
	rm -rf /tmp/kubernetes