.PHONY: all

all: $(ROOTFS)/etc/dns/bootstrap.sh

$(ROOTFS)/etc/dns/bootstrap.sh:
	@echo "\n---> Bootstrapping DNS \n"
	cp -af $(ASSETS)/makefiles/master/dns/bootstrap.service $(ROOTFS)/lib/systemd/system/dns-bootstrap.service
	ln -sf /lib/systemd/system/dns-bootstrap.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/
	mkdir -p $(ROOTFS)/etc/dns/
	install -m 0755 ./bootstrap.sh $(ROOTFS)/etc/dns/
