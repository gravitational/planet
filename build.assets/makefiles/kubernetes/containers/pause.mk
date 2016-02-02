.PHONY: all export pull-from-internet

IMAGE:=gcr.io/google_containers/pause:0.8.0
EXPORTDIR:=$(BUILD_ASSETS)/k8s-$(KUBE_VER)/containers
OUT:=$(EXPORTDIR)/pause.tar.gz

all: pull-from-internet $(OUT)

$(OUT):
	@echo "Exporting image to file system..."
	@mkdir -p $(EXPORTDIR)
	docker save -o $@ $(IMAGE)

# TODO: make this target the result of `docker ps | grep nettest`
pull-from-internet:
	@echo "Pulling docker image..."
	docker pull $(IMAGE)
