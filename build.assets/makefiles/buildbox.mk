# This makefile runs on the host and it uses buildbox Docker image
# to kick off inside-buildbox building
SHELL:=/bin/bash
TARGETDIR:=$(BUILDDIR)/$(TARGET)
ROOTFS:=$(TARGETDIR)/rootfs
CONTAINERNAME:=planet-base-$(TARGET)
TARBALL:=$(TARGETDIR)/planet-$(TARGET).$(PLANETVER).tar.gz

.PHONY: all clean

# invoke "TARGET-docker.mk" from inside of 'buildbox' docker image:
all: $(ROOTFS)/bin/bash 
	@echo -e "\\n---> Launching 'buildbox' Docker container to build $(TARGET):\\n"
	docker run -ti --rm=true \
		--volume=$(ASSETS):/assets \
		--volume=$(ROOTFS):/rootfs \
		--volume=$(TARGETDIR):/targetdir \
		--volume=$(PWD):/gopath/src/github.com/gravitational/planet \
		--env="ASSETS=/assets"\
		--env="ROOTFS=/rootfs" \
		--env="TARGETDIR=/targetdir" \
		--env="TARGET=$(TARGET)" \
		planet/buildbox \
		make -f assets/makefiles/$(TARGET)-docker.mk
	cp $(ASSETS)/orbit.manifest.json $(TARGETDIR)
	@echo -e "\\n---> Moving current symlink to $(TARGETDIR)\\n"
	@rm -f $(BUILDDIR)/current
	@cd $(BUILDDIR) && ln -fs $(TARGET) $(BUILDDIR)/current
	@echo -e "\\n---> Creating Planet image...\\n"
	cd $(TARGETDIR) && tar -czf $(TARBALL) orbit.manifest.json rootfs
	@echo -e "\\nDone --> $(TARBALL)"


$(ROOTFS)/bin/bash: clean-rootfs
	@echo -e "\\n---> Creating RootFS for Planet image:\\n"
	@mkdir -p $(ROOTFS)
# create rootfs based in RAM. you can uncomment the next line to use disk
	sudo mount -t tmpfs -o size=600m tmpfs $(ROOTFS)
# populate Rootfs using docker image 'planet/base'
	docker create --name=$(CONTAINERNAME) planet/base
	@echo "Exporting base Docker image into a fresh RootFS into $(ROOTFS)...."
	cd $(ROOTFS) && docker export $(CONTAINERNAME) | tar -x


clean-rootfs:
# umount tmps volume for rootfs:
	if [[ $$(mount | grep $(ROOTFS)) ]]; then \
		sudo umount -f $(ROOTFS) 2>/dev/null ;\
	fi
# delete docker container we've used to create rootfs:
	if [[ $$(docker ps -a | grep $(CONTAINERNAME)) ]]; then \
		docker rm -f $(CONTAINERNAME) ;\
	fi

clean: clean-rootfs
	sudo rm -rf $(TARGETDIR)
