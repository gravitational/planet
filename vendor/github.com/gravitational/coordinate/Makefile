.PHONY: test

TEST_ETCD_IMAGE := quay.io/coreos/etcd:v2.3.7
TEST_ETCD_INSTANCE := coordinate0

test:
	if docker ps | grep $(TEST_ETCD_INSTANCE) --quiet; then \
	  echo "ETCD is already running"; \
	else \
	  echo "starting test ETCD instance"; \
	  etcd_instance=$(shell docker ps -a | grep $(TEST_ETCD_INSTANCE) | awk '{print $$1}'); \
	  if [ "$$etcd_instance" != "" ]; then \
	    docker rm -v $$etcd_instance; \
	  fi; \
	  docker run --name=$(TEST_ETCD_INSTANCE) -p 34001:4001 -p 32380:2380 -p 32379:2379 -d $(TEST_ETCD_IMAGE) -name etcd0 -listen-client-urls=http://0.0.0.0:2379,http://0.0.0.0:4001 -advertise-client-urls http://localhost:32379,http://localhost:34001; \
	fi;
	COORDINATE_TEST_ETCD_NODES=http://127.0.0.1:34001 go test -v -test.parallel=0 ./...

