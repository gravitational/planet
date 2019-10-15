FROM centos:7

ARG GOVERSION=go1.12.9

ENV GOPATH /gopath
ENV GOROOT /opt/go
ENV PATH $PATH:$GOPATH/bin:$GOROOT/bin
ENV GOCACHE ${GOPATH}/.gocache-${GOVERSION}

# Have our own /etc/passwd with users populated from 990 to 1000
COPY passwd /etc/passwd

# Install build tools, dev tools and Go:
RUN yum install -y glibc-devel systemd-devel \
    curl make git gcc tar gzip
RUN mkdir -p /opt && cd /opt && curl https://storage.googleapis.com/golang/go$GOVERSION.linux-amd64.tar.gz | tar xz
RUN mkdir -p $GOPATH/src $GOPATH/bin ${GOCACHE};go get github.com/tools/godep
RUN go get github.com/gravitational/version/cmd/linkflags
RUN chmod a+w $GOPATH -R
RUN chmod a+w $GOROOT -R
