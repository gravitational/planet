.PHONY: all image

BUILDDIR := $(abspath build)
export

all:
	mkdir -p $(BUILDDIR)
	$(MAKE) -C image -f image.mk
	$(MAKE) -C rkt -f rkt.mk
	$(MAKE) -C etcd -f etcd.mk

run:
	sudo systemd-nspawn --boot --capability=all --register=true --uuid=51dbfeb9-59f9-4a5b-82db-0e5924202c63 --machine=cube -D $(BUILDDIR)/rootfs

enter:
	sudo nsenter --target $$(machinectl status cube | grep Leader | grep -Po '\d+') --mount --uts --ipc --net /bin/bash





