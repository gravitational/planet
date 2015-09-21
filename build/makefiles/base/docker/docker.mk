.PHONY: all

ARCH := x86_64
OS := Linux
VER := 1.6.2

$(ROOTFS)/usr/bin/docker: $(OUT)/docker
	# install socket-activated metadata service
	cp -af ./docker.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/docker.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/

	# install the unmount cleanup script
	mkdir -p $(ROOTFS)/usr/bin/scripts
	install -m 0755 ./unmount-devmapper.sh $(ROOTFS)/usr/bin/scripts

	# copy docker from the build dir into rootfs:
	mkdir -p $(ROOTFS)/usr/bin
	cp $(OUT)/docker $(ROOTFS)/usr/bin/docker

$(OUT)/docker:
	# download release
	curl -L https://get.docker.com/builds/$(OS)/$(ARCH)/docker-$(VER) -o $(OUT)/docker
	chmod +x $(OUT)/docker
