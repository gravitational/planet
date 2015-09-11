.PHONY: all

VER := 92f21b3fe3399ae6e1c29979649ba7e64d6d096e

BINARIES := $(BUILDDIR)/kube-apiserver $(BUILDDIR)/kube-controller-manager $(BUILDDIR)/kube-scheduler $(BUILDDIR)/kubectl $(BUILDDIR)/kube-proxy $(BUILDDIR)/kubelet

all: kubernetes.mk $(BINARIES)

$(BINARIES): DIR := $(shell mktemp -d)
$(BINARIES): GOPATH := $(DIR)
$(BINARIES):
	mkdir -p $(DIR)/src/github.com/kubernetes
	cd $(DIR)/src/github.com/kubernetes && git clone https://github.com/kubernetes/kubernetes
	cd $(DIR)/src/github.com/kubernetes/kubernetes && git checkout $(VER)
	cd $(DIR)/src/github.com/kubernetes/kubernetes && ./hack/build-go.sh
	cp $(DIR)/src/github.com/kubernetes/kubernetes/_output/local/bin/linux/amd64/kube* $(BUILDDIR)
	rm -rf $(DIR)
