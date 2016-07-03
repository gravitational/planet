# This makefile installs Kubernetes-friendly docker inside of planet
# images. 
# 
# This makefile itself is executed inside the docker's buildbox image.
.PHONY: all

ARCH := x86_64
OS := Linux
VER := 1.9.1
BINARIES := $(ASSETDIR)/docker-$(VER)

$(ROOTFS)/usr/bin/docker: $(BINARIES)
# install socket-activated metadata service
	@echo "\n---> Installing Docker to be used with Kubernetes:\n"
	cp -af ./docker.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/docker.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/
	cp -af ./docker.socket $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/docker.socket $(ROOTFS)/lib/systemd/system/sockets.target.wants/

# install the unmount cleanup script
	mkdir -p $(ROOTFS)/usr/bin/scripts
	install -m 0755 ./unmount-devmapper.sh $(ROOTFS)/usr/bin/scripts

# copy docker from the build dir into rootfs:
	mkdir -p $(ROOTFS)/usr/bin
	cp $(BINARIES) $(ROOTFS)/usr/bin/docker

$(BINARIES):
# download release
	curl -L https://get.docker.com/builds/$(OS)/$(ARCH)/docker-$(VER) -o $@
	chmod +x $@
