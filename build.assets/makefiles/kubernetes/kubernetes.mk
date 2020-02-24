.PHONY: all

CURL_OPTS := --retry 100 --retry-delay 0 --connect-timeout 10 --max-time 300 --tlsv1.2 --silent --show-error
DOWNLOAD_URL := https://storage.googleapis.com/kubernetes-release/release/$(KUBE_VER)/bin/linux/amd64
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
	sleep 2 # downloads on gcloud seem to fail if hitting the server too quickly
