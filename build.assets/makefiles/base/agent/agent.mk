.PHONY: all

all:
	@echo "\n---> Installing services for Planet agent:\n"

	cp -af ./planet-agent.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/planet-agent.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/
