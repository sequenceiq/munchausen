FROM gliderlabs/alpine

MAINTAINER SequenceIQ <info@sequenceiq.com>

RUN apk update && apk add go && rm -rf /var/cache/apk/*

ENV GOROOT /usr/lib/go
ENV GOPATH /go
ENV PATH $PATH:$GOROOT/bin:$GOPATH/bin

COPY . /go/src/github.com/sequenceiq/swarm-bootstrap
WORKDIR /go/src/github.com/sequenceiq/swarm-bootstrap

ENV GOPATH /go/src/github.com/sequenceiq/swarm-bootstrap/Godeps/_workspace:$GOPATH
RUN CGO_ENABLED=0 go install -v -a

ENTRYPOINT ["swarm-bootstrap"]