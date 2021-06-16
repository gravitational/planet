.PHONY: all pull-from-internet

IMAGE:=k8s.gcr.io/nettest:1.9
# OUTDIR defines the output directory for the resulting tarball
# (set in the parent makefile)
override OUT:=$(OUTDIR)/nettest.tar.gz

all: pull-from-internet $(OUT)

$(OUT): nettest.mk
	@echo "Exporting image to file system..."
	docker save -o $@ $(IMAGE)

pull-from-internet:
	@echo "Pulling docker image..."
	docker pull $(IMAGE)
