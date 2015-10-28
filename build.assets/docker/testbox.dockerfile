# Docker image with kubernetes repository used for e2e testing
FROM debian:jessie

ENV GOPATH /gopath
ENV KUBE_VER e3188f6ee7007000c5daf525c8cc32b4c5bf4ba8

RUN apt-get -q -y update && apt-get install -q -y git

# Install Kubernetes:
RUN mkdir -p $GOPATH/src/github.com/kubernetes; \
	cd $GOPATH/src/github.com/kubernetes; \
	git clone https://github.com/kubernetes/kubernetes && cd kubernetes && git checkout $KUBE_VER
