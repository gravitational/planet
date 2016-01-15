.PHONY: all

VER := v0.7.0
REPODIR := $(GOPATH)/src/github.com/hashicorp/serf
OUT := $(ASSETDIR)/serf-$(VER)
BINARIES := $(ROOTFS)/usr/bin/serf

all: agent.mk $(OUT) $(BINARIES)

$(OUT):
	@echo "\n---> Building Serf:\n"
	mkdir -p $(GOPATH)/src/github.com/hashicorp
	cd $(GOPATH)/src/github.com/hashicorp && git clone https://github.com/hashicorp/serf -b $(VER) --depth 1 
	cd $(REPODIR) && \
	go get -t -d ./... && \
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $@ .

$(BINARIES):
	@echo "\n---> Installing services for Serf/Planet agent:\n"
	cp $(OUT) $@
	cp -af ./serf.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/serf.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/
	cp -af ./agent.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/agent.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/
