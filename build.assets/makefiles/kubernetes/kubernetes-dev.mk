.PHONY: all

VER := e3188f6ee7007000c5daf525c8cc32b4c5bf4ba8

BINARIES := $(TARGETDIR)/kube-apiserver \
	$(TARGETDIR)/kube-controller-manager \
	$(TARGETDIR)/kube-scheduler \
	$(TARGETDIR)/kubectl \
	$(TARGETDIR)/kube-proxy \
	$(TARGETDIR)/kubelet \
	$(TARGETDIR)/test/ginkgo \
	$(TARGETDIR)/test/e2e.test

all: kubernetes-dev.mk $(BINARIES)

$(BINARIES): GOPATH := /gopath
$(BINARIES):
	@echo "\n---> Building Kubernetes\n"
	mkdir -p $(GOPATH)/src/github.com/kubernetes
	cd $(GOPATH)/src/github.com/kubernetes && git clone https://github.com/kubernetes/kubernetes
	cd $(GOPATH)/src/github.com/kubernetes/kubernetes && git checkout $(VER)
	cd $(GOPATH)/src/github.com/kubernetes/kubernetes && ./hack/build-go.sh
	cp $(GOPATH)/src/github.com/kubernetes/kubernetes/_output/local/bin/linux/amd64/kube* $(TARGETDIR)/
	cp $(GOPATH)/src/github.com/kubernetes/kubernetes/_output/local/bin/linux/amd64/ginkgo $(TARGETDIR)/test/
	cp $(GOPATH)/src/github.com/kubernetes/kubernetes/_output/local/bin/linux/amd64/e2e.test $(TARGETDIR)/test/
