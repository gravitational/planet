.PHONY: all

REPODIR := $(GOPATH)/src/github.com/kubernetes/kubernetes
OUTPUTDIR := $(ASSETDIR)/k8s-$(KUBE_VER)
BINARIES := $(OUTPUTDIR)/kube-apiserver \
	$(OUTPUTDIR)/kube-controller-manager \
	$(OUTPUTDIR)/kube-scheduler \
	$(OUTPUTDIR)/kubectl \
	$(OUTPUTDIR)/kube-proxy \
	$(OUTPUTDIR)/kubelet \
	$(OUTPUTDIR)/ginkgo \
	$(OUTPUTDIR)/e2e.test

all: kubernetes.mk $(BINARIES)

$(BINARIES): GOPATH := /gopath
$(BINARIES):
	@echo "\n---> Building Kubernetes:\n"
	mkdir -p $(OUTPUTDIR)
	mkdir -p $(GOPATH)/src/github.com/kubernetes
	cd $(GOPATH)/src/github.com/kubernetes && git clone https://github.com/kubernetes/kubernetes -b $(KUBE_VER) --depth 1 
	cd $(REPODIR) && GO15VENDOREXPERIMENT= ./hack/build-go.sh
	cp $(REPODIR)/_output/local/bin/linux/amd64/kube* $(OUTPUTDIR)/
	cp $(REPODIR)/_output/local/bin/linux/amd64/ginkgo $(OUTPUTDIR)/
	cp $(REPODIR)/_output/local/bin/linux/amd64/e2e.test $(OUTPUTDIR)/
