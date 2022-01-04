.PHONY: all

OUT := $(ASSETDIR)/serf-$(SERF_VER)
BINARIES := $(ROOTFS)/usr/bin/serf

all: agent.mk $(OUT) $(BINARIES)

$(OUT):
	@echo "\n---> Building Serf:\n"
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go install github.com/hashicorp/serf/cmd/serf@$(SERF_VER)
	cp $(GOPATH)/bin/serf $@

$(BINARIES): serf.service planet-agent.service
	@echo "\n---> Installing services for Serf/Planet agent:\n"
	cp $(OUT) $@
	cp -af ./serf.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/serf.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/
	cp -af ./planet-agent.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/planet-agent.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/
