.PHONY: all

CURL_OPTS := -s
DOWNLOAD_URL := https://storage.googleapis.com/kubernetes-release/release/$(KUBE_VER)/bin/linux/amd64
REPODIR := $(GOPATH)/src/github.com/kubernetes/kubernetes
OUTPUTDIR := $(ASSETDIR)/k8s-$(KUBE_VER)
HELM_TARBALL:= $(ASSETDIR)/helm-$(HELM_VER).tgz
COREDNS_TARBALL := $(ASSETDIR)/coredns-$(COREDNS_VER).tgz
BINARIES := kube-apiserver \
	kube-controller-manager \
	kube-scheduler \
	kubectl \
	kube-proxy \
	kubelet
KUBE_OUTPUTS := $(addprefix $(OUTPUTDIR)/, $(BINARIES))
OUTPUTS := $(KUBE_OUTPUTS) $(HELM_TARBALL) $(COREDNS_TARBALL)

all: kubernetes.mk $(OUTPUTS) | $(DIRS)
	tar xvzf $(COREDNS_TARBALL) --strip-components=1 -C $(ROOTFS)/usr/bin coredns
	tar xvzf $(HELM_TARBALL) --strip-components=1 -C $(ROOTFS)/usr/bin linux-amd64/helm

$(DIRS):
	mkdir -p $(OUTPUTDIR) $(HELM_OUTPUTDIR) $(COREDNS_OUTPUTDIR)

$(KUBE_OUTPUTS):
	@echo "\n---> Downloading kubernetes:\n"
	curl $(CURL_OPTS) -o $@ $(DOWNLOAD_URL)/$(notdir $@)
	chmod +x $@

$(HELM_TARBALL):
	curl https://kubernetes-helm.storage.googleapis.com/helm-v$(HELM_VER)-linux-amd64.tar.gz \
		-o $@

$(COREDNS_TARBALL):
	curl -L https://github.com/coredns/coredns/releases/download/v${COREDNS_VER}/coredns_${COREDNS_VER}_linux_amd64.tgz \
		-o $@
