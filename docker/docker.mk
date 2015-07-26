.PHONY: all patch-stage1

all: docker.mk
# install socket-activated metadata service
	cp -af ./docker.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/docker.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/
# cleanup existing scripts that we don't need
	rm -f $(ROOTFS)/etc/init.d/docker
	rm -f $(ROOTFS)/lib/systemd/system/docker.socket
	rm -f $(ROOTFS)/lib/systemd/system/sockets.target.wants/docker.socket
# install the unmount cleanup script
	mkdir -p $(ROOTFS)/usr/bin/scripts
	install -m 0755 ./unmount-devmapper.sh $(ROOTFS)/usr/bin/scripts

