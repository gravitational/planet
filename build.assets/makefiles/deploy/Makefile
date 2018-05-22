# Deploy build artifacts to Amazon S3
# Prefix used to place build artifacts on Amazon S3
# The artifacts are stored $BUILD_BUCKET_URL/planet/$(PLANET_BUILD_TAG)/planet.tar.gz
# with BUILD_TAG derived from `git describe`
BUILD_BUCKET_URL ?= s3://clientbuilds.gravitational.io/planet
REPO_URL := quay.io/gravitational
TARGET ?= planet
ROOTFS := $(OUTPUTDIR)/rootfs

ifndef BUILDDIR
$(error must specify BUILDDIR)
endif

ifndef PLANET_BUILD_TAG
$(error must specify PLANET_BUILD_TAG)
endif

REMOTE_IMAGE := $(REPO_URL)/planet:$(PLANET_BUILD_TAG)

.PHONY: all
all: deploy

.PHONY: deploy
deploy: deploy-s3 deploy-quay

.PHONY: image
image:
	cp $(ASSETS)/docker/export.dockerfile $(OUTPUTDIR)
	cd $(OUTPUTDIR) && docker build -t $(REMOTE_IMAGE) \
		-f export.dockerfile .
	@-rm $(OUTPUTDIR)/export.dockerfile

.PHONY: deploy-s3
deploy-s3:
	@echo "Deploying $(PLANET_BUILD_TAG) to $(BUILD_BUCKET_URL)"
	aws s3 cp $(BUILDDIR)/planet.tar.gz \
		$(BUILD_BUCKET_URL)/$(PLANET_BUILD_TAG)/

.PHONY: deploy-quay
deploy-quay: image
	docker tag $(PLANET_IMAGE) $(REMOTE_IMAGE)
	docker push $(REMOTE_IMAGE)
