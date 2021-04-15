ARG GOVERSION=go1.12.17-stretch

FROM quay.io/gravitational/debian-venti:${GOVERSION}

ENV PATH $PATH:$GOPATH/bin:$GOROOT/bin
ENV GOCACHE ${GOPATH}/.gocache-${GOVERSION}

RUN apt-get update && apt-get install -y libc6-dev libudev-dev

RUN mkdir -p $GOPATH/src/github.com/{gravitational,hashicorp} $GOPATH/src/github.com/hashicorp/serf $GOPATH/bin ${GOCACHE};go get github.com/tools/godep
RUN go get github.com/gravitational/version/cmd/linkflags
RUN chmod a+w $GOPATH -R
RUN chmod a+w $GOROOT -R
