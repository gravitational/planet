SHELL:=/bin/bash
.PHONY: all image etcd network k8s-master planet notary

MASTER_IP := 54.149.35.97
NODE_IP := 54.149.186.124
NODE2_IP := 54.68.41.110
BUILDDIR := $(HOME)/build
export

all: planet-base planet-master planet-node planet notary

notary:
	$(MAKE) -C makefiles/notary -f notary.mk

dev: planet-dev
	cd $(BUILDDIR) && tar -xzf planet-dev.aci

planet:
	go build -o $(BUILDDIR)/planet github.com/gravitational/planet/planet
	go build -o $(BUILDDIR)/rootfs/usr/bin/planet github.com/gravitational/planet/planet

planet-inside:
	go build -o /var/orbit/render/gravitational.com/planet-dev/0.0.2/c61cb786ea66c515c090477eb15b0807c8aa89d64eece0785e61a0a849366d30/rootfs/usr/bin/planet  github.com/gravitational/planet/planet

planet-base:
	sudo docker build --no-cache=true -t planet/base -f makefiles/base/base.dockerfile .

os-image:
	sudo docker build --no-cache=true -t planet/os -f makefiles/os/os.dockerfile . ;\

planet-dev:
	sudo docker build -t planet/dev -f makefiles/dev/dev.dockerfile .
	mkdir -p $(BUILDDIR)
	rm -rf $(BUILDDIR)/planet-dev.aci
	id=$$(sudo docker create planet/dev:latest) && sudo docker cp $$id:/build/planet-dev.aci $(BUILDDIR)

planet-master:
	sudo docker build -t planet/master -f makefiles/master/master.dockerfile .
	mkdir -p $(BUILDDIR)
	rm -rf $(BUILDDIR)/planet-master.aci
	id=$$(sudo docker create planet/master:latest) && sudo docker cp $$id:/build/planet-master.aci $(BUILDDIR)

planet-node:
	sudo docker build -t planet/node -f makefiles/node/node.dockerfile .
	mkdir -p $(BUILDDIR)
	rm -rf $(BUILDDIR)/planet-node.aci
	id=$$(sudo docker create planet/node:latest) && sudo docker cp $$id:/build/planet-node.aci $(BUILDDIR)/

kill-systemd:
	sudo kill -9 $$(ps uax  | grep [/]bin/systemd | awk '{ print $$2 }')

login-master:
	ssh -i /home/alex/keys/aws/alex.pem ubuntu@$(MASTER_IP)

login-node:
	ssh -i /home/alex/keys/aws/alex.pem ubuntu@$(NODE_IP)

login-node2:
	ssh -i /home/alex/keys/aws/alex.pem ubuntu@$(NODE2_IP)

deploy-master:
	scp -i /home/alex/keys/aws/alex.pem  $($(BUILDDIR)DIR)/kube-master.tar.gz ubuntu@$(MASTER_IP):/home/ubuntu

deploy-experiment:
	scp -i /home/alex/keys/aws/alex.pem start.sh ubuntu@$(MASTER_IP):/home/ubuntu

deploy-node:
	scp -i /home/alex/keys/aws/alex.pem  $($(BUILDDIR)DIR)/planet-node.tar.gz ubuntu@$(NODE_IP):/home/ubuntu

deploy-node2:
	scp -i /home/alex/keys/aws/alex.pem  $($(BUILDDIR)DIR)/planet-node.tar.gz ubuntu@$(NODE2_IP):/home/ubuntu

deploy-master:
	scp -i /home/alex/keys/aws/alex.pem  $($(BUILDDIR)DIR)/planet ubuntu@$(MASTER_IP):/home/ubuntu/

deploy-node:
	scp -i /home/alex/keys/aws/alex.pem  $($(BUILDDIR)DIR)/planet ubuntu@$(NODE_IP):/home/ubuntu

deploy-nsenter:
	scp -i /home/alex/keys/aws/alex.pem /usr/bin/nsenter ubuntu@$(MASTER_IP):/home/ubuntu/

deploy-nsenter-node:
	scp -i /home/alex/keys/aws/alex.pem /usr/bin/nsenter ubuntu@$(NODE_IP):/home/ubuntu/

deploy-nsenter-node2:
	scp -i /home/alex/keys/aws/alex.pem /usr/bin/nsenter ubuntu@$(NODE2_IP):/home/ubuntu/

deploy-kubectl:
	scp -i /home/alex/keys/aws/alex.pem $($(BUILDDIR)DIR)/kubectl ubuntu@$(MASTER_IP):/home/ubuntu/

# IMPORTANT NOTES for installer
# * We need to set cloud provider for kubernetes - semi done, aws
# * Flanneld needs NET_ADMIN and modpropbe, so we need to mount /lib/modules - done
# * Kube-node needs master private IP - done

# Have a unified way to generate environment for master and node in a consistent way and use one file everywhere -done
# what's the problem with udevd (turn it off probably) ?
# cgroups should be mounted in systemd compatible way (cpu,cpuacct)

# kernel version on ubuntu 14.04, docker with overlayfs needs new kernel. Devicemapper is not stable.
# sudo apt-get install linux-headers-generic-lts-vivid linux-image-generic-lts-vivid
# check kernel version, and if it's less than 3.18 back off

enter:
	sudo $(BUILDDIR)/rootfs/usr/bin/planet enter --debug $(BUILDDIR)/rootfs


clean:
	rm -rf $(BUILDDIR)/master/rootfs/run/planet.socket


start-dev:
	sudo mkdir -p /var/planet/registry /var/planet/etcd /var/planet/docker /var/planet/mysql
	sudo $(BUILDDIR)/rootfs/usr/bin/planet start\
		--role=master\
		--role=node\
		--volume=/var/planet/etcd:/ext/etcd\
		--volume=/var/planet/registry:/ext/registry\
		--volume=/var/planet/docker:/ext/docker\
        --volume=/var/planet/mysql:/ext/mysql\
		$(BUILDDIR)/rootfs

stop:
	sudo $(BUILDDIR)/rootfs/usr/bin/planet stop $(BUILDDIR)/rootfs

status:
	sudo $(BUILDDIR)/rootfs/usr/bin/planet status $(BUILDDIR)/rootfs

