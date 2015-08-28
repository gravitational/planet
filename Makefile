.PHONY: all image etcd network k8s-master cube notary

MASTER_IP := 54.149.35.97
NODE_IP := 54.149.186.124
NODE2_IP := 54.68.41.110
BUILDDIR := $(HOME)/build
export

all: cube-base cube-master cube-node cube notary

notary:
	$(MAKE) -C makefiles/notary -f notary.mk

dev:
	cd $(BUILDDIR) && tar -xzf cube-dev.aci

cube:
	go build -o $(BUILDDIR)/cube github.com/gravitational/cube/cube
	go build -o $(BUILDDIR)/rootfs/usr/bin/cube github.com/gravitational/cube/cube

cube-inside:
	go build -o /var/orbit/render/gravitational.com/cube-dev/0.0.2/c61cb786ea66c515c090477eb15b0807c8aa89d64eece0785e61a0a849366d30/rootfs/usr/bin/cube  github.com/gravitational/cube/cube

cube-os:
	sudo docker build --no-cache=true -t cube/os -f makefiles/cube-os/cube-os.dockerfile .

cube-base:
	sudo docker build --no-cache=true -t cube/base -f makefiles/cube-base/cube-base.dockerfile .

cube-dev:
	sudo docker build -t cube/dev -f makefiles/cube-dev/cube-dev.dockerfile .
	mkdir -p $(BUILDDIR)
	rm -rf $(BUILDDIR)/cube-dev.aci
	id=$$(sudo docker create cube/dev:latest) && sudo docker cp $$id:/build/cube-dev.aci $(BUILDDIR)

cube-master:
	sudo docker build -t cube/master -f makefiles/cube-master/cube-master.dockerfile .
	mkdir -p $(BUILDDIR)
	rm -rf $(BUILDDIR)/cube-master.aci
	id=$$(sudo docker create cube/master:latest) && sudo docker cp $$id:/build/cube-master.aci $(BUILDDIR)

cube-node:
	sudo docker build -t cube/node -f makefiles/cube-node/cube-node.dockerfile .
	mkdir -p $(BUILDDIR)
	rm -rf $(BUILDDIR)/cube-node.aci
	id=$$(sudo docker create cube/node:latest) && sudo docker cp $$id:/build/cube-node.aci $(BUILDDIR)/

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
#	scp -i /home/alex/keys/aws/alex.pem  ./image.tar.gz ubuntu@$(MASTER_IP):/home/ubuntu

deploy-node:
	scp -i /home/alex/keys/aws/alex.pem  $($(BUILDDIR)DIR)/kube-node.tar.gz ubuntu@$(NODE_IP):/home/ubuntu

deploy-node2:
	scp -i /home/alex/keys/aws/alex.pem  $($(BUILDDIR)DIR)/kube-node.tar.gz ubuntu@$(NODE2_IP):/home/ubuntu

deploy-cube-master:
	scp -i /home/alex/keys/aws/alex.pem  $($(BUILDDIR)DIR)/cube ubuntu@$(MASTER_IP):/home/ubuntu/

deploy-cube-node:
	scp -i /home/alex/keys/aws/alex.pem  $($(BUILDDIR)DIR)/cube ubuntu@$(NODE_IP):/home/ubuntu

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
	sudo $(BUILDDIR)/rootfs/usr/bin/cube enter --debug $(BUILDDIR)/rootfs


clean:
	rm -rf $(BUILDDIR)/kube-master/rootfs/run/cube.socket


start-dev:
	sudo mkdir -p /var/cube/registry /var/cube/etcd /var/cube/docker /var/cube/mysql
	sudo $(BUILDDIR)/rootfs/usr/bin/cube start\
		--role=master\
		--role=node\
		--volume=/var/cube/etcd:/ext/etcd\
		--volume=/var/cube/registry:/ext/registry\
		--volume=/var/cube/docker:/ext/docker\
        --volume=/var/cube/mysql:/ext/mysql\
		$(BUILDDIR)/rootfs

stop:
	sudo $(BUILDDIR)/rootfs/usr/bin/cube stop $(BUILDDIR)/rootfs

status:
	sudo $(BUILDDIR)/rootfs/usr/bin/cube status $(BUILDDIR)/rootfs

