.PHONY: all

all: build
	@echo "\\n---> Installing DNS resolution service:\\n"

	# Install CoreDNS
	mkdir -p $(ROOTFS)/etc/coredns/configmaps/ $(ROOTFS)/usr/lib/sysusers.d/
	cp -af ./coredns.service $(ROOTFS)/lib/systemd/system/
	ln -sf /lib/systemd/system/coredns.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/

build:
	@echo "\\n---> Building CoreDNS:\\n"
	# Temporary, build from fork until https://github.com/coredns/coredns/issues/2116 is fixed upstream
	mkdir -p $(GOPATH)/src/github.com/coredns
	cd $(GOPATH)/src/github.com/coredns && git clone https://github.com/gravitational/coredns -b kevin/unblock-startup --depth 1 
	$(MAKE) -C $(GOPATH)/src/github.com/coredns/coredns/ coredns
	cp -af $(GOPATH)/src/github.com/coredns/coredns/coredns $(ROOTFS)/usr/bin