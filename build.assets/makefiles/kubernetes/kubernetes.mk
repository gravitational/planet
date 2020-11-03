.PHONY: all

CURL_OPTS := --retry 100 --retry-delay 0 --connect-timeout 10 --max-time 300 --tlsv1.2 --silent --show-error
DOWNLOAD_URL := https://storage.googleapis.com/kubernetes-release/release/$(KUBE_VER)/bin/linux/amd64
REPODIR := $(GOPATH)/src/github.com/kubernetes/kubernetes
OUTPUTDIR := $(ASSETDIR)/k8s-$(KUBE_VER)
HELM_TARBALL:= $(ASSETDIR)/helm-$(HELM_VER).tgz
HELM3_TARBALL:= $(ASSETDIR)/helm-$(HELM3_VER).tgz
COREDNS_TARBALL := $(ASSETDIR)/coredns-$(COREDNS_VER).tgz
BINARIES := kube-apiserver \
	kube-controller-manager \
	kube-scheduler \
	kubectl \
	kube-proxy \
	kubelet
KUBE_OUTPUTS := $(addprefix $(OUTPUTDIR)/, $(BINARIES))
OUTPUTS := $(KUBE_OUTPUTS) $(HELM_TARBALL) $(HELM3_TARBALL) $(COREDNS_TARBALL)

all: kubernetes.mk $(OUTPUTS)
	tar xvzf $(COREDNS_TARBALL) -C $(ROOTFS)/usr/bin coredns
	tar xvzf $(HELM_TARBALL) --strip-components=1 -C $(ROOTFS)/usr/bin linux-amd64/helm
	tar --transform='flags=r;s|helm|helm3|' -xvzf $(HELM3_TARBALL) --strip-components=1 -C $(ROOTFS)/usr/bin linux-amd64/helm

$(OUTPUTDIR):
	mkdir -p $@

$(KUBE_OUTPUTS): | $(OUTPUTDIR)
	@echo "\n---> Downloading kubernetes:\n"
	curl $(CURL_OPTS) -o $@ $(DOWNLOAD_URL)/$(notdir $@)
	chmod +x $@

$(HELM_TARBALL):
	curl https://get.helm.sh/helm-v$(HELM_VER)-linux-amd64.tar.gz \
		-o $@

$(HELM3_TARBALL):
	curl https://get.helm.sh/helm-v$(HELM3_VER)-linux-amd64.tar.gz \
		-o $@

$(COREDNS_TARBALL):
	curl -L https://github.com/coredns/coredns/releases/download/v${COREDNS_VER}/coredns_${COREDNS_VER}_linux_amd64.tgz \
		-o $@
