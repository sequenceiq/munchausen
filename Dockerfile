FROM gliderlabs/alpine:3.1

MAINTAINER SequenceIQ <info@sequenceiq.com>

COPY . /go/src/github.com/sequenceiq/swarm-bootstrap
WORKDIR /go/src/github.com/sequenceiq/swarm-bootstrap

RUN apk-install -t build-deps go git \
    && cd /go/src/github.com/sequenceiq/swarm-bootstrap \
    && export GOPATH=/go \
    && export PATH=$PATH:/$GOPATH/bin \
    && go get github.com/tools/godep \
    && godep restore \
    && go build -o /bin/swarm-bootstrap \
    && rm -rf /go \
    && apk del --purge build-deps

ENTRYPOINT ["/bin/swarm-bootstrap"]