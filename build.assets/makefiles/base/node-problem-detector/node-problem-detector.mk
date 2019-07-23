.PHONY: all

DIR_NAME := node-problem-detector-$(NODE_PROBLEM_DETECTOR_VER)
REPO_URL := https://github.com/kubernetes/node-problem-detector
DOWNLOAD_URL := $(REPO_URL)/releases/download/$(NODE_PROBLEM_DETECTOR_VER)/$(DIR_NAME).tar.gz
CURL_FLAGS := -L --retry 5
OUT_DIR := $(ASSETDIR)/$(DIR_NAME)
OUTS := $(OUT_DIR)/bin/node-problem-detector $(OUT_DIR)/bin/log-counter
TARGETS := $(ROOTFS)/usr/bin/node-problem-detector $(ROOTFS)/usr/bin/log-counter

all: node-problem-detector.mk $(OUTS) $(TARGETS)

$(OUTS):
	@echo "\n---> Downloading Node Problem Detector $(NODE_PROBLEM_DETECTOR_VER):\n"
	mkdir -p $(OUT_DIR)
	curl $(CURL_FLAGS) $(DOWNLOAD_URL) | tar xz -C $(OUT_DIR)

$(TARGETS):
	@echo "\n---> Installing Node Problem Detector $(NODE_PROBLEM_DETECTOR_VER):\n"
	cp -af $(OUTS) $(ROOTFS)/usr/bin
	cp -af ./node-problem-detector.service $(ROOTFS)/lib/systemd/system
	ln -sf /lib/systemd/system/node-problem-detector.service $(ROOTFS)/lib/systemd/system/multi-user.target.wants/
