FROM golang:1.3

COPY . /go/src/github.com/sequenceiq/swarm-bootstrap
WORKDIR /go/src/github.com/sequenceiq/swarm-bootstrap

ENV GOPATH /go/src/github.com/sequenceiq/swarm-bootstrap/Godeps/_workspace:$GOPATH
RUN go install -v -a

ENTRYPOINT ["swarm-bootstrap"]