FROM diogomonica/golang-softhsm2
MAINTAINER Diogo Monica "diogo@docker.com"

# CHANGE-ME: Default values for SoftHSM2 PIN and SOPIN, used to initialize the first token
ENV NOTARY_SIGNER_PIN="1234"
ENV SOPIN="1234"
ENV LIBDIR="/usr/local/lib/softhsm/"
ENV NOTARY_SIGNER_DEFAULT_ALIAS="timestamp_1"
ENV NOTARY_SIGNER_TIMESTAMP_1="testpassword"

# Install openSC and dependencies
RUN apt-get update && \
    apt-get install -y \
    libltdl-dev \
    libpcsclite-dev \
    opensc \
    usbutils \
    --no-install-recommends \
    && rm -rf /var/lib/apt/lists/*

# Initialize the SoftHSM2 token on slod 0, using PIN and SOPIN varaibles
RUN softhsm2-util --init-token --slot 0 --label "test_token" --pin $NOTARY_SIGNER_PIN --so-pin $SOPIN

ENV NOTARYPKG github.com/docker/notary
ENV GOPATH /go/src/${NOTARYPKG}/Godeps/_workspace:$GOPATH

EXPOSE 4443

RUN mkdir -p /go/src/github.com/docker/ && cd /go/src/github.com/docker/ && git clone https://github.com/docker/notary.git && cd notary && git checkout bf2831f3a53ca3afe319f049950eacd77774602c
ADD . /config

WORKDIR /go/src/${NOTARYPKG}

# Install notary-signer
RUN go install \
    -ldflags "-w -X ${NOTARYPKG}/version.GitCommit `git rev-parse --short HEAD` -X ${NOTARYPKG}/version.NotaryVersion `cat NOTARY_VERSION`" \
    ${NOTARYPKG}/cmd/notary-signer


ENTRYPOINT [ "notary-signer" ]
CMD [ "-config=/config/signer-config.json", "-debug" ]
