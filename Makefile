.PHONY: all image etcd network k8s-master cube

MASTER_IP := 54.149.35.97
NODE_IP := 54.149.186.124
NODE2_IP := 54.68.41.110
BUILDDIR := $(abspath build)
export

all: cube-base cube-master cube-node cube

cube:
	go build -o build/cube github.com/gravitational/cube/cube

cube-base:
	cd makefiles/cube-base && sudo docker build -t cube/base .

cube-dev:
	cd makefiles && sudo docker build -t cube/dev .
	mkdir -p build
	rm -rf build/kube-dev.tar.gz
	id=$$(sudo docker create cube/dev:latest) && sudo docker cp $$id:/build/kube-dev.tar.gz build/

cube-master:
	cd makefiles/cube-master && sudo docker build -t cube/master .
	mkdir -p build
	rm -rf build/kube-master.tar.gz
	id=$$(sudo docker create cube/dev:latest) && sudo docker cp $$id:/build/kube-dev.tar.gz build/

cube-node:
	cd makefiles/cube-node && sudo docker build -t cube/node .
	mkdir -p build
	rm -rf build/kube-node.tar.gz
	id=$$(sudo docker create cube/node:latest) && sudo docker cp $$id:/build/kube-node.tar.gz build/

kill-systemd:
	sudo kill -9 $$(ps uax  | grep [/]bin/systemd | awk '{ print $$2 }')

login-master:
	ssh -i /home/alex/keys/aws/alex.pem ubuntu@$(MASTER_IP)

login-node:
	ssh -i /home/alex/keys/aws/alex.pem ubuntu@$(NODE_IP)

login-node2:
	ssh -i /home/alex/keys/aws/alex.pem ubuntu@$(NODE2_IP)

deploy-master:
	scp -i /home/alex/keys/aws/alex.pem  $(BUILDDIR)/kube-master.tar.gz ubuntu@$(MASTER_IP):/home/ubuntu

deploy-experiment:
	scp -i /home/alex/keys/aws/alex.pem start.sh ubuntu@$(MASTER_IP):/home/ubuntu
#	scp -i /home/alex/keys/aws/alex.pem  ./image.tar.gz ubuntu@$(MASTER_IP):/home/ubuntu

deploy-node:
	scp -i /home/alex/keys/aws/alex.pem  $(BUILDDIR)/kube-node.tar.gz ubuntu@$(NODE_IP):/home/ubuntu

deploy-node2:
	scp -i /home/alex/keys/aws/alex.pem  $(BUILDDIR)/kube-node.tar.gz ubuntu@$(NODE2_IP):/home/ubuntu

deploy-cube-master:
	scp -i /home/alex/keys/aws/alex.pem  $(BUILDDIR)/cube ubuntu@$(MASTER_IP):/home/ubuntu/

deploy-cube-node:
	scp -i /home/alex/keys/aws/alex.pem  $(BUILDDIR)/cube ubuntu@$(NODE_IP):/home/ubuntu

deploy-nsenter:
	scp -i /home/alex/keys/aws/alex.pem /usr/bin/nsenter ubuntu@$(MASTER_IP):/home/ubuntu/

deploy-nsenter-node:
	scp -i /home/alex/keys/aws/alex.pem /usr/bin/nsenter ubuntu@$(NODE_IP):/home/ubuntu/

deploy-nsenter-node2:
	scp -i /home/alex/keys/aws/alex.pem /usr/bin/nsenter ubuntu@$(NODE2_IP):/home/ubuntu/

deploy-kubectl:
	scp -i /home/alex/keys/aws/alex.pem $(BUILDDIR)/kubectl ubuntu@$(MASTER_IP):/home/ubuntu/

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
	sudo ./build/cube enter build/rootfs


clean:
	rm -rf build/kube-master/rootfs/run/cube.socket


start-dev:
	sudo mkdir -p /var/cube/registry /var/cube/etcd
	sudo ./build/cube start -role=master\
          -role=node -volume=/var/cube/etcd:/var/etcd\
          -volume=/var/cube/registry:/var/registry build/rootfs

stop:
	sudo ./build/cube stop build/rootfs
