.PHONY: all

VER := e3188f6ee7007000c5daf525c8cc32b4c5bf4ba8
BINARIES := $(TARGETDIR)/kube-apiserver $(TARGETDIR)/kube-controller-manager $(TARGETDIR)/kube-scheduler $(TARGETDIR)/kubectl $(TARGETDIR)/kube-proxy $(TARGETDIR)/kubelet

all: kubernetes.mk $(BINARIES)

$(BINARIES): DIR := $(shell mktemp -d)
$(BINARIES): GOPATH := $(DIR)
$(BINARIES):
	@echo "\\n---> Building Kubernetes\\n"
	mkdir -p $(DIR)/src/github.com/kubernetes
	cd $(DIR)/src/github.com/kubernetes && git clone https://github.com/kubernetes/kubernetes
	cd $(DIR)/src/github.com/kubernetes/kubernetes && git checkout $(VER)
	cd $(DIR)/src/github.com/kubernetes/kubernetes && ./hack/build-go.sh
	cp $(DIR)/src/github.com/kubernetes/kubernetes/_output/local/bin/linux/amd64/kube* $(TARGETDIR)/
	rm -rf $(DIR)
