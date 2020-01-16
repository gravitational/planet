FROM quay.io/gravitational/debian-venti:go1.12.9-buster

ARG GOVERSION=1.12.9

ENV PATH $PATH:$GOPATH/bin:$GOROOT/bin
ENV GOCACHE ${GOPATH}/.gocache-${GOVERSION}

# Have our own /etc/passwd with users populated from 990 to 1000
COPY passwd /etc/passwd

RUN apt-get update && apt-get install -y libc6-dev libudev-dev

RUN mkdir -p $GOPATH/src $GOPATH/bin ${GOCACHE};go get github.com/tools/godep
RUN go get github.com/gravitational/version/cmd/linkflags
RUN chmod a+w $GOPATH -R
RUN chmod a+w $GOROOT -R
