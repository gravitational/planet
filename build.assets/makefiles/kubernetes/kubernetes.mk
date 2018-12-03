.PHONY: all

CURL_OPTS := -s
DOWNLOAD_URL := https://s3-us-west-2.amazonaws.com/dev.gravitational.io/kubernetes-release/release/$(KUBE_VER)/linux/amd64
REPODIR := $(GOPATH)/src/github.com/kubernetes/kubernetes
OUTPUTDIR := $(ASSETDIR)/k8s-$(KUBE_VER)
BINARIES := kube-apiserver \
	kube-controller-manager \
	kube-scheduler \
	kubectl \
	kube-proxy \
	kubelet
OUTPUTS := $(addprefix $(OUTPUTDIR)/, $(BINARIES))

all: kubernetes.mk $(OUTPUTS)

$(OUTPUTS):
	@echo "\n---> Downloading kubernetes:\n"
	mkdir -p $(OUTPUTDIR)
	curl $(CURL_OPTS) -o $@ $(DOWNLOAD_URL)/$(notdir $@)
	chmod +x $@
