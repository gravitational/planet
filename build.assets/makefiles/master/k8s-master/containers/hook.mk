.PHONY: all build docker clean

ifndef KUBE_VER
$(error KUBE_VER not set)
endif

VER:=0.0.1
TAG:=$(VER)
IMAGE:=gravitational.io/hook:$(TAG)
# OUTDIR defines the output directory for the resulting tarball
# (set in the parent makefile)
override OUT:=$(OUTDIR)/hook.tar.gz

all: docker $(OUT)

$(OUT): hook.mk
	@echo "Exporting $(IMAGE) to file system..."
	docker save -o $@ $(IMAGE)

# build the container
docker: hook.dockerfile
	docker build -t $(IMAGE) --build-arg KUBE_VER=$(KUBE_VER) -f hook.dockerfile .

clean:
	-docker rmi $(IMAGE)
	-rm build/kubectl
