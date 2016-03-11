# Deploy build artifacts to Amazon S3
#
.PHONY: all deploy

TARGETS=master node
# Prefix used to place build artifacts on Amazon S3
# The artifacts are stored as s3://builds.gravitational.io/planet/$(BUILD_TAG)/planet-$(TARGET).tar.gz
# with BUILD_TAG derived from `git describe`
BUILD_BUCKET_URL=s3://builds.gravitational.io/planet
BUILD_TAG=$(shell git describe)

all: deploy

#
# Prepare awslci configuration for jenkins. Only used on jenkins!
# When deploying from local machine, follow the Build deployment configuration
# section in gravity README
#
deploy-jenkins:
	-cp deploy_artifacts.ini $(HOME)/.aws/

deploy:
	@echo "Deploying $(BUILD_TAG) to Amazon S3"
	$(foreach target,\
		$(TARGETS),\
		AWS_CONFIG_FILE=./deploy_artifacts.ini aws s3 cp \
		$(BUILDDIR)/$(target)/planet-$(target).tar.gz \
		$(BUILD_BUCKET_URL)/$(BUILD_TAG)/ --profile artifacts;)
