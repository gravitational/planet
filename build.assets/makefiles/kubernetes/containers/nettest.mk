.PHONY: all export pull-from-internet

IMAGE:=gcr.io/google_containers/nettest:1.6
EXPORTDIR:=$(BUILD_ASSETS)/k8s-$(KUBE_VER)/containers
OUT:=$(EXPORTDIR)/nettest.tar.gz

all: pull-from-internet $(OUT)

$(OUT):
	@echo "Exporting image to file system..."
	@mkdir -p $(EXPORTDIR)
	docker save -o $@ $(IMAGE)

pull-from-internet:
	@echo "Pulling docker image..."
	docker pull $(IMAGE)
