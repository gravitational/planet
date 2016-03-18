.PHONY: all pull-from-internet

IMAGE:=gcr.io/google_containers/nettest:1.8
# OUTDIR defines the output directory for the resulting tarball
# (set in the parent makefile)
override OUT:=$(OUTDIR)/nettest.tar.gz

all: pull-from-internet $(OUT)

$(OUT):
	@echo "Exporting image to file system..."
	docker save -o $@ $(IMAGE)

pull-from-internet:
	@echo "Pulling docker image..."
	docker pull $(IMAGE)
