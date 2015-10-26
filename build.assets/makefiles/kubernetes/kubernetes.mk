.PHONY: all

BINARIES := $(TARGETDIR)/kube-apiserver \
	$(TARGETDIR)/kube-controller-manager \
	$(TARGETDIR)/kube-scheduler \
	$(TARGETDIR)/kubectl \
	$(TARGETDIR)/kube-proxy \
	$(TARGETDIR)/kubelet \
	$(TARGETDIR)/test/e2e.test \
	$(TARGETDIR)/test/ginkgo

all: kubernetes.mk $(BINARIES)

$(BINARIES): GOPATH := /gopath
$(BINARIES):
	@echo "GOPATH: ${GOPATH}\n"
	@echo "\n---> Building Kubernetes\n"
	cd $(GOPATH)/src/github.com/kubernetes/kubernetes && ./hack/build-go.sh
	cp $(GOPATH)/src/github.com/kubernetes/kubernetes/_output/local/bin/linux/amd64/kube* $(TARGETDIR)/
	mkdir -p $(TARGETDIR)/test
	cp $(GOPATH)/src/github.com/kubernetes/kubernetes/_output/local/bin/linux/amd64/ginkgo $(TARGETDIR)/test/
	cp $(GOPATH)/src/github.com/kubernetes/kubernetes/_output/local/bin/linux/amd64/e2e.test $(TARGETDIR)/test/
