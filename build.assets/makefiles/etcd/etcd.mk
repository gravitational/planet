MAKEFILE := $(lastword $(MAKEFILE_LIST))
OS := linux
ARCH := amd64
TARBALLS := $(addprefix $(ASSETDIR)/etcd-,$(addsuffix -$(OS)-$(ARCH).tar.gz,$(ETCD_VER)))
server_targets = $(addprefix $(ROOTFS)/usr/bin/etcd-,$(ETCD_VER))
ctl_targets = $(addprefix $(ROOTFS)/usr/bin/etcdctl-cmd-,$(ETCD_VER))

# outputs parameters with a prefix and a suffix new line
define print
	@echo "" && echo $1 && echo ""
endef

.PHONY: all
all: require-ASSETDIR require-ETCD_VER require-ROOTFS
all: $(server_targets) $(ctl_targets)
	$(call print, "---> Building etcd:")
	$(call print, "---> Setup etcd services:")
	cd $(ASSETDIR)
	cp -afv ./etcd.service $(ROOTFS)/lib/systemd/system/
	cp -afv ./etcd-upgrade.service $(ROOTFS)/lib/systemd/system/
	cp -afv ./etcd-gateway.dropin $(ROOTFS)/lib/systemd/system/
	cp -afv ./etcdctl3 $(ROOTFS)/usr/bin/etcdctl3
	cp -afv ./etcdctl $(ROOTFS)/usr/bin/etcdctl
	chmod +x $(ROOTFS)/usr/bin/etcdctl3 $(ROOTFS)/usr/bin/etcdctl
	ln -sf /lib/systemd/system/etcd.service $(ROOTFS)/lib/systemd/system/multi-user.target.wants/

	# mask the etcd-upgrade service so that it can only be run if intentionally unmasked
	ln -sf /dev/null $(ROOTFS)/etc/systemd/system/etcd-upgrade.service

	# Write to the release file to indicate the latest release
	echo PLANET_ETCD_VERSION=$(ETCD_LATEST_VER) >> $(ROOTFS)/etc/planet-release

$(TARBALLS): $(ASSETDIR)/etcd-%-$(OS)-$(ARCH).tar.gz:
	$(call print, "---> $@ - Downloading etcd")
	curl -L https://github.com/coreos/etcd/releases/download/$*/$(@F) -o $@

$(server_targets): $(ROOTFS)/usr/bin/etcd-%: $(TARBALLS)
	$(call print, "---> $@ - Extracting etcd")
	tar xf $(ASSETDIR)/etcd-$*-$(OS)-$(ARCH).tar.gz \
		--strip-components 1 \
		--directory $(@D) \
		--transform="s|etcd$$|$(@F)|" \
		etcd-$*-$(OS)-$(ARCH)/etcd

$(ctl_targets): $(ROOTFS)/usr/bin/etcdctl-cmd-%: $(TARBALLS)
	$(call print, "---> $@ - Extracting etcdctl")
	tar xf $(ASSETDIR)/etcd-$*-$(OS)-$(ARCH).tar.gz \
		--strip-components 1 \
		--directory $(@D) \
		--transform="s|etcdctl$$|$(@F)|" \
		etcd-$*-$(OS)-$(ARCH)/etcdctl

require-%: FORCE
	@if [ -z '${${*}}' ]; then echo 'Environment variable $* not set.' && exit 1; fi

.PHONY: FORCE
FORCE:
