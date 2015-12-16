.PHONY: all

OUTPUTDIR:=$(ASSETDIR)/k8s-$(KUBE_VER)

all: k8s-master.mk
	@echo "\n---> Building master k8s component (kube-apiserver, kube-scheduler, kube-controller-manager)\n"
	cp -af ./kube-apiserver.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/kube-apiserver.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/
	cp -af ./kube-controller-manager.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/kube-controller-manager.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/
	cp -af ./kube-scheduler.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/kube-scheduler.service  $(ROOTFS)/lib/systemd/system/multi-user.target.wants/
	cp ./*.conf $(ROOTFS)/lib/monit/init
	install -m 0755 $(OUTPUTDIR)/kube-apiserver $(ROOTFS)/usr/bin
	install -m 0755 $(OUTPUTDIR)/kube-controller-manager $(ROOTFS)/usr/bin
	install -m 0755 $(OUTPUTDIR)/kube-scheduler $(ROOTFS)/usr/bin
	install -m 0755 $(OUTPUTDIR)/kubectl $(ROOTFS)/usr/bin
