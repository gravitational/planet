.PHONY: all

REPODIR := $(GOPATH)/src/github.com/kubernetes/kubernetes
BINARIES := $(ASSETDIR)/kube-apiserver \
	$(ASSETDIR)/kube-controller-manager \
	$(ASSETDIR)/kube-scheduler \
	$(ASSETDIR)/kubectl \
	$(ASSETDIR)/kube-proxy \
	$(ASSETDIR)/kubelet \
	$(ASSETDIR)/ginkgo \
	$(ASSETDIR)/e2e.test

all: kubernetes.mk $(BINARIES)

$(BINARIES): GOPATH := /gopath
$(BINARIES):
	@echo "\n---> Building Kubernetes:\n"
	mkdir -p $(GOPATH)/src/github.com/kubernetes
	cd $(GOPATH)/src/github.com/kubernetes && git clone https://github.com/kubernetes/kubernetes -b $(KUBE_VER) --depth 1 
	cd $(REPODIR) && ./hack/build-go.sh
	cp $(REPODIR)/_output/local/bin/linux/amd64/kube* $(ASSETDIR)/
	cp $(REPODIR)/_output/local/bin/linux/amd64/ginkgo $(ASSETDIR)/
	cp $(REPODIR)/_output/local/bin/linux/amd64/e2e.test $(ASSETDIR)/
