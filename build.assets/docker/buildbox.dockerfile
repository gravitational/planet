FROM planet/base

ENV GOPATH /gopath
ENV GOROOT /opt/go
ENV PATH $PATH:$GOPATH/bin:$GOROOT/bin
# FIXME: moved here to pull kubernetes once when the image is built
ENV KUBE_VER e3188f6ee7007000c5daf525c8cc32b4c5bf4ba8

# Install build tools, dev tools and Go:
RUN apt-get update && apt-get install -y curl make git libc6-dev gcc tar gzip vim screen
RUN mkdir -p /opt && cd /opt && curl https://storage.googleapis.com/golang/go1.4.2.linux-amd64.tar.gz | tar xz
RUN mkdir -p $GOPATH/src/github.com/kubernetes $GOPATH/bin; \
	go get github.com/tools/godep; \
	cd $GOPATH/src/github.com/kubernetes; \
	git clone https://github.com/kubernetes/kubernetes && cd kubernetes && git checkout $KUBE_VER
