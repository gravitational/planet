MAKEFILE := $(lastword $(MAKEFILE_LIST))
OS := linux
ARCH := amd64
targets = $(addprefix $(ROOTFS)/usr/bin/etcd-, $(ETCD_VER))

# outputs parameters with a prefix and a suffix new line
define print
	@echo "" && echo $1 && echo ""
endef

.PHONY: all
all: require-ASSETDIR require-ETCD_VER require-ROOTFS
all: $(targets)
	$(call print, "---> Building etcd:")
	$(call print, "---> Setup etcd services:")
	cd $(ASSETDIR)
	cp -afv ./etcd.service $(ROOTFS)/lib/systemd/system/
	cp -afv ./etcd-upgrade.service $(ROOTFS)/lib/systemd/system/
	cp -afv ./etcd-gateway.dropin $(ROOTFS)/lib/systemd/system/
	cp -afv ./etcdctl3 $(ROOTFS)/usr/bin/etcdctl3
	chmod +x $(ROOTFS)/usr/bin/etcdctl3
	ln -sf /lib/systemd/system/etcd.service $(ROOTFS)/lib/systemd/system/multi-user.target.wants/

	# mask the etcd-upgrade service so that it can only be run if intentionally unmasked
	ln -sf /dev/null $(ROOTFS)/etc/systemd/system/etcd-upgrade.service

	# Write to the release file to indicate the latest release
	echo PLANET_ETCD_VERSION=$(ETCD_LATEST_VER) >> $(ROOTFS)/etc/planet-release

.PHONY: $(ETCD_VER)
$(ETCD_VER): output=etcd-$@-$(OS)-$(ARCH).tar.gz
$(ETCD_VER): targetdir=$(ASSETDIR)/etcd-$@-$(OS)-$(ARCH)
$(ETCD_VER):
	$(call print, "---> $@ - Downloading etcd")
	curl -L https://github.com/coreos/etcd/releases/download/$@/$(output) \
		-o $(ASSETDIR)/$(output)

	$(call print, "---> $@ - Extracting etcd")
	tar -xzf $(ASSETDIR)/$(output) -C $(ASSETDIR)

	cp -afv $(targetdir)/etcd $(ROOTFS)/usr/bin/etcd-$@
	cp -afv $(targetdir)/etcdctl $(ROOTFS)/usr/bin/etcdctl-$@


$(targets): version=$(lastword $(subst -, ,$(notdir $@)))
$(targets):
	$(MAKE) -f $(MAKEFILE) $(version)

require-%: FORCE
	@if [ -z '${${*}}' ]; then echo 'Environment variable $* not set.' && exit 1; fi

.PHONY: FORCE
FORCE:
