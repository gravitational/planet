# Docker image with kubernetes repository used for e2e testing
FROM debian:jessie

ENV GOPATH /gopath
ENV KUBE_VER v1.0.7

RUN apt-get -q -y update && apt-get install -q -y git

# Install Kubernetes:
RUN mkdir -p $GOPATH/src/github.com/kubernetes; \
	cd $GOPATH/src/github.com/kubernetes; \
	git clone https://github.com/kubernetes/kubernetes && cd kubernetes && git checkout $KUBE_VER
