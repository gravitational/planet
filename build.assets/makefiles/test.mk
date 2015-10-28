# Runs end-to-end test using a testbox image
BIN:=$(BUILDDIR)/$(TARGET)/rootfs/usr/bin
TEST_BIN:=$(BUILDDIR)/$(TARGET)/test
TEST_ASSETS:=$(PWD)/test/e2e
KUBE_HOME:=/gopath/src/github.com/kubernetes/kubernetes
KUBE_MASTER:=127.0.0.1:8080

.PHONY: all

all: test.mk $(BINARIES)
	@echo $(TESTDIR)
	@echo -e "\n---> Launching 'testbox' for end-to-end tests:\n"
	docker run -ti --rm=true \
		--net=host \
		--volume=$(ASSETS):/assets \
		--volume=$(BIN):/bindir \
		--volume=$(TEST_ASSETS):/test \
		--volume=$(TEST_BIN):/test/bin \
		--env="ASSETS=/assets" \
		--env="TARGETDIR=/targetdir" \
		planet/testbox \
		/bindir/planet test \
			--kube-master=$(KUBE_MASTER) \
			--tool-dir=/test/bin \
			--kube-repo=$(KUBE_HOME) \
			--kube-config=/test/kubeconfig -- -focus=Network
	@echo -e "\nDone"
