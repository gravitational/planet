.PHONY: all

BINARIES := $(TARGETDIR)/kube-apiserver \
	$(TARGETDIR)/kube-controller-manager \
	$(TARGETDIR)/kube-scheduler \
	$(TARGETDIR)/kubectl \
	$(TARGETDIR)/kube-proxy \
	$(TARGETDIR)/kubelet

all: kubernetes.mk $(BINARIES)

$(BINARIES): GOPATH := /gopath
$(BINARIES):
	@echo "\n---> Building Kubernetes\n"
	mkdir -p $(GOPATH)/src/github.com/kubernetes
	cd $(GOPATH)/src/github.com/kubernetes && git clone https://github.com/kubernetes/kubernetes
	cd $(GOPATH)/src/github.com/kubernetes/kubernetes && git checkout $(KUBE_VER)
	cd $(GOPATH)/src/github.com/kubernetes/kubernetes && ./hack/build-go.sh
	cp $(GOPATH)/src/github.com/kubernetes/kubernetes/_output/local/bin/linux/amd64/kube* $(TARGETDIR)/
