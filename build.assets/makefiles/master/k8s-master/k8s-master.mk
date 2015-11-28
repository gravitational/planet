.PHONY: all

all: k8s-master.mk
	@echo "\n---> Building master k8s component (kube-apiserver, kube-scheduler, kube-controller-manager)\n"
	cp -af ./kube-apiserver.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/kube-apiserver.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/
	cp -af ./kube-controller-manager.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/kube-controller-manager.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/
	cp -af ./kube-scheduler.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/kube-scheduler.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/
	cp ./*.conf $(ROOTFS)/lib/monit/init
	install -m 0755 $(TARGETDIR)/kube-apiserver $(ROOTFS)/usr/bin
	install -m 0755 $(TARGETDIR)/kube-controller-manager $(ROOTFS)/usr/bin
	install -m 0755 $(TARGETDIR)/kube-scheduler $(ROOTFS)/usr/bin
	install -m 0755 $(TARGETDIR)/kubectl $(ROOTFS)/usr/bin
