FROM golang

RUN apt-get update && apt-get install -y \
    libltdl-dev \
    --no-install-recommends \
    && rm -rf /var/lib/apt/lists/*

EXPOSE 4443

ENV NOTARYPKG github.com/docker/notary
ENV GOPATH /go/src/${NOTARYPKG}/Godeps/_workspace:$GOPATH

RUN mkdir -p /go/src/github.com/docker/ && cd /go/src/github.com/docker/ && git clone https://github.com/docker/notary.git && cd notary && git checkout bf2831f3a53ca3afe319f049950eacd77774602c
ADD . /config

WORKDIR /go/src/${NOTARYPKG}

RUN go install \
    -ldflags "-w -X ${NOTARYPKG}/version.GitCommit `git rev-parse --short HEAD` -X ${NOTARYPKG}/version.NotaryVersion `cat NOTARY_VERSION`" \
    ${NOTARYPKG}/cmd/notary-server

ENTRYPOINT [ "notary-server" ]
CMD [ "-config", "/config/server-config.json" ]
