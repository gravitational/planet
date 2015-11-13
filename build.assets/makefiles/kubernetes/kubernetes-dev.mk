.PHONY: all

GOPATH := /gopath
REPODIR := $(GOPATH)/src/github.com/kubernetes/kubernetes


BINARIES := $(TARGETDIR)/kube-apiserver \
	$(TARGETDIR)/kube-controller-manager \
	$(TARGETDIR)/kube-scheduler \
	$(TARGETDIR)/kubectl \
	$(TARGETDIR)/kube-proxy \
	$(TARGETDIR)/kubelet \
	$(TARGETDIR)/ginkgo \
	$(TARGETDIR)/e2e.test

all: kubernetes-dev.mk $(BINARIES)

$(BINARIES):
	@echo "\n---> Building Kubernetes\n"
	mkdir -p $(GOPATH)/src/github.com/kubernetes
	cd $(GOPATH)/src/github.com/kubernetes && git clone https://github.com/kubernetes/kubernetes
	cd $(REPODIR) && git checkout $(KUBE_VER)
	$(REPODIR)/hack/build-go.sh
	cp $(REPODIR)/_output/local/bin/linux/amd64/kube* $(TARGETDIR)/
	cp $(REPODIR)/_output/local/bin/linux/amd64/ginkgo $(TARGETDIR)/
	cp $(REPODIR)/_output/local/bin/linux/amd64/e2e.test $(TARGETDIR)/
