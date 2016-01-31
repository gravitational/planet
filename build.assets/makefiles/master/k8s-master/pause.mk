.PHONY: all start-registry push-to-registry build-image export-from-registry make-tarball stop-registry

CONTAINER=pause:0.8.0

REGISTRYIMAGE=registry:2.1.1
REGISTRYPORT=5055
RUNNINGREG=$$(docker ps -q --filter=image=$(REGISTRYIMAGE))
REGADDRESS=127.0.0.1:$(REGISTRYPORT)
BINDIR:=$(ASSETDIR)/k8s-$(KUBE_VER)
OUT=build/assets/k8s-$(KUBE_VER)/pause.tar.gz

all: $(OUT)

$(OUT): 
	@echo "\n---> Building infra-POD namespace/IPC container\n"
	$(MAKE) start-registry
	$(MAKE) build-image
	$(MAKE) push-to-registry
	$(MAKE) export-from-registry
	$(MAKE) make-tarball
	$(MAKE) stop-registry


make-tarball:
	@echo "Making a tarball...."
	mkdir -p build
	tar -cvzf $(OUT) resources registry
	@echo "done ---> $(OUT)"

clean:
	$(MAKE) stop-registry
	rm -rf build
	rm -rf registry

push-to-registry:
	@echo "Pushing image to registry..."
	@echo "docker tag $(CONTAINER) $(REGADDRESS)/$(CONTAINER)" ;\
	docker tag $(CONTAINER) $(REGADDRESS)/$(CONTAINER) ;\
	docker push $(REGADDRESS)/$(CONTAINER) ;\
	docker rmi $(REGADDRESS)/$(CONTAINER) ;\

export-from-registry:
	@echo "Exporting image from registry to file system..."
	@rm -rf registry
	docker cp $(RUNNINGREG):/var/lib/registry registry


start-registry:
	@if [ -z "$(RUNNINGREG)" ]; then \
		docker run -d -p $(REGISTRYPORT):5000 $(REGISTRYIMAGE) ;\
		echo "docker registry running on $(REGISTRYPORT)\n" ;\
	else \
		echo "docker registry already listening on $(REGISTRYPORT)\n" ;\
	fi

stop-registry:
	@if [ ! -z "$(RUNNINGREG)" ]; then \
		docker kill $(RUNNINGREG) ;\
	else \
		echo registry is not running ;\
	fi

# maybe pull from internet
pull-from-internet:
	@echo "Pulling docker image..."
	docker pull $(CONTAINER) ;\
