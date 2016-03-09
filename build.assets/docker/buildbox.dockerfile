FROM planet/base

ENV GOPATH /gopath
ENV GOROOT /opt/go
ENV PATH $PATH:$GOPATH/bin:$GOROOT/bin
ENV GO15VENDOREXPERIMENT 1

# Have our own /etc/passwd with users populated from 990 to 1000
COPY passwd /etc/passwd

# Install build tools, dev tools and Go:
RUN apt-get update && apt-get install -y curl make git libc6-dev libudev-dev gcc tar gzip vim screen
RUN mkdir -p /opt && cd /opt && curl https://storage.googleapis.com/golang/go1.5.2.linux-amd64.tar.gz | tar xz
RUN mkdir -p $GOPATH/src $GOPATH/bin;go get github.com/tools/godep
RUN go get github.com/gravitational/version/cmd/linkflags
RUN chmod a+w $GOPATH -R
RUN chmod a+w $GOROOT -R
