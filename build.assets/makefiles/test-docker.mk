# This makefile runs iside of docker's buildbox
# The following volumes are mounted and shared with the host:
TEST_HOME := /targetdir/test
KUBE_CONFIG := /test/kubeconfig
KUBE_HOME := /gopath/src/github.com/kubernetes/kubernetes
KUBE_MASTER := 127.0.0.1
BINARIES := $(TEST_HOME)/e2e.test $(TEST_HOME)/ginkgo
export TEST_HOME
export KUBE_CONFIG
export KUBE_HOME
export KUBE_MASTER

all: $(BINARIES)
	/test/e2e-test.sh
