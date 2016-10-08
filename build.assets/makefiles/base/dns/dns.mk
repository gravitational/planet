.PHONY: all

all:
	@echo "\\n---> Installing DNS resolution service:\\n"
	cp -af ./dnsmasq.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/dnsmasq.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/

# script that allows setting up dnsmasq
	mkdir -p $(ROOTFS)/usr/bin/scripts
	install -m 0755 ./dnsmasq.sh $(ROOTFS)/usr/bin/scripts

# script that sets up /etc/hosts and symlinks resolv.conf
	mkdir -p $(ROOTFS)/etc/dnsmasq.d
