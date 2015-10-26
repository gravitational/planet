# Runs end-to-end using a buildbox docker image
SHELL:=/bin/bash
TARGETDIR:=$(BUILDDIR)/$(TARGET)
TESTDIR:=$(PWD)/test/e2e

.PHONY: all

all: test.mk $(BINARIES)
	@echo $(TESTDIR)
	@echo -e "\n---> Launching 'buildbox' for end-to-end tests:\n"
	docker run -ti --rm=true \
		--net=host \
		--volume=$(ASSETS):/assets \
		--volume=$(TARGETDIR):/targetdir \
		--volume=$(TESTDIR):/test \
		--env="ASSETS=/assets" \
		--env="TARGETDIR=/targetdir" \
		planet/buildbox \
		make -f assets/makefiles/test-docker.mk
	@echo -e "\nDone --> $(TARBALL)"
