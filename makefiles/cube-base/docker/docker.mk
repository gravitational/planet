.PHONY: all

ARCH := x86_64
OS := Linux
VER := 1.8.0

all: docker.mk
# install socket-activated metadata service
	cp -af ./docker.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/docker.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/

# install the unmount cleanup script
	mkdir -p $(ROOTFS)/usr/bin/scripts
	install -m 0755 ./unmount-devmapper.sh $(ROOTFS)/usr/bin/scripts

# download release
	mkdir -p $(ROOTFS)/usr/bin
	curl -L https://get.docker.com/builds/$(OS)/$(ARCH)/docker-$(VER) -o $(ROOTFS)/usr/bin/docker
	chmod +x $(ROOTFS)/usr/bin/docker

