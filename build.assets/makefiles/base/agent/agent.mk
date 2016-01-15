.PHONY: all

all: 
	@echo "\\n---> Installing Planet Agent service:\\n"

	cp -af ./planet-agent.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/planet-agent.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/
