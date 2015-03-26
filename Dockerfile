FROM gliderlabs/alpine:3.1

MAINTAINER SequenceIQ <info@sequenceiq.com>

COPY . /go/src/github.com/sequenceiq/swarm-bootstrap
WORKDIR /go/src/github.com/sequenceiq/swarm-bootstrap

RUN apk-install -t build-deps go git \
    && cd /go/src/github.com/sequenceiq/swarm-bootstrap \
    && export GOPATH=/go \
    && go get \
    && go build -o /bin/swarm-bootstrap \
    && rm -rf /go \
    && apk del --purge build-deps

ENTRYPOINT ["/bin/swarm-bootstrap"]