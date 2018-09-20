.PHONY: all

all:
	@echo "\\n---> Installing DNS resolution service:\\n"

	# Install CoreDNS
	mkdir -p $(ROOTFS)/etc/coredns/configmaps/ $(ROOTFS)/usr/lib/sysusers.d/
	cp -af ./coredns.service $(ROOTFS)/lib/systemd/system/
	ln -sf /lib/systemd/system/coredns.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/
