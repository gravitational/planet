.PHONY: all

all:
	@echo "\\n---> Installing DNS resolution service:\\n"
#	cp -af ./dnsmasq.service $(ROOTFS)/lib/systemd/system
#	ln -sf /lib/systemd/system/dnsmasq.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/

# script that sets up /etc/hosts and symlinks resolv.conf
#	mkdir -p $(ROOTFS)/etc/dnsmasq.d

	# Install COREDNS
	mkdir -p $(ROOTFS)/etc/coredns/configmaps/ $(ROOTFS)/usr/lib/sysusers.d/
	cp -af ./coredns.service $(ROOTFS)/lib/systemd/system/
