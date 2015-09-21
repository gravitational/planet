FROM planet/base

ENV GOPATH /gopath
ENV GOROOT /opt/go
ENV PATH $PATH:$GOPATH/bin:$GOROOT/bin
ENV BUILDDIR /build

# Install build tools, dev tools and Golang:
RUN apt-get install -y curl make git libc6-dev gcc tar gzip vim screen
RUN mkdir -p /opt && cd /opt && curl https://storage.googleapis.com/golang/go1.4.2.linux-amd64.tar.gz | tar xz
RUN mkdir -p $GOPATH/src $GOPATH/bin;go get github.com/tools/godep


VOLUME "/build"
