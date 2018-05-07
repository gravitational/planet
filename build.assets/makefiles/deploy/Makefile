# Deploy build artifacts to Amazon S3
# Prefix used to place build artifacts on Amazon S3
# The artifacts are stored $BUILD_BUCKET_URL/planet/$(BUILD_TAG)/planet-$(TARGET).tar.gz
# with BUILD_TAG derived from `git describe`
BUILD_BUCKET_URL ?= s3://clientbuilds.gravitational.io/planet
REPO_URL := quay.io/gravitational

ifndef BUILDDIR
$(error must specify BUILDDIR)
endif

ifndef PLANET_IMAGE
$(error must specify PLANET_IMAGE)
endif

.PHONY: all
all: deploy-s3 deploy-quay

.PHONY: deploy-s3
deploy-s3:
	@echo "Deploying $(BUILD_TAG) to $(BUILD_BUCKET_URL)"
	aws s3 cp $(BUILDDIR)/planet.tar.gz \
		$(BUILD_BUCKET_URL)/$(BUILD_TAG)/

.PHONY: deploy-quay
deploy-quay:
	docker tag $(PLANET_IMAGE) $(REPO_URL)/$(PLANET_IMAGE)
	docker push $(REPO_URL)/$(PLANET_IMAGE)
