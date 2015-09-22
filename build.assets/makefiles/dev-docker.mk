# This makefile runs iside of docker's buildbox
# The following volumes are mounted and shared with the host:
ASSETS := /assets
ROOTFS := /rootfs
TARGETDIR := /targetdir

all:
# common components:
	make -C $(ASSETS)/makefiles/base/network -f network.mk
	make -C $(ASSETS)/makefiles/base/docker -f docker.mk 
	make -C $(ASSETS)/makefiles/registry -f registry.mk
	make -C $(ASSETS)/makefiles/kubernetes -f kubernetes.mk
# dev-image specific:
	make -C $(ASSETS)/makefiles/master/etcd -f etcd.mk
	make -C $(ASSETS)/makefiles/node/k8s-node -f k8s-node.mk
	make -C $(ASSETS)/makefiles/master/k8s-master -f k8s-master.mk
