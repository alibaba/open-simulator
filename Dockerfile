FROM golang:1.18.3 AS builder

WORKDIR /go/src/github.com/alibaba/open-simulator
COPY . .
RUN make test
RUN make build