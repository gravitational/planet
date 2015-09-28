# This makefile installs Kubernetes-friendly docker inside of planet
# images. 
# 
# This makefiel itself is executed inside of docker's buildbox image.
.PHONY: all

ARCH := x86_64
OS := Linux
VER := 1.8.2

$(ROOTFS)/usr/bin/docker: $(TARGETDIR)/docker
# install socket-activated metadata service
	@echo "\\n---> Insatlling Docker to be used with Kubernetes:\\n"
	cp -af ./docker.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/docker.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/

# install the unmount cleanup script
	mkdir -p $(ROOTFS)/usr/bin/scripts
	install -m 0755 ./unmount-devmapper.sh $(ROOTFS)/usr/bin/scripts

# copy docker from the build dir into rootfs:
	mkdir -p $(ROOTFS)/usr/bin
	cp $(TARGETDIR)/docker $(ROOTFS)/usr/bin/docker

$(TARGETDIR)/docker:
# download release
	curl -L https://get.docker.com/builds/$(OS)/$(ARCH)/docker-$(VER) -o $(TARGETDIR)/docker
	chmod +x $(TARGETDIR)/docker
