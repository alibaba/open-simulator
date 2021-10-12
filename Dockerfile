FROM golang:1.15.6 AS builder

WORKDIR /go/src/github.com/alibaba/open-simulator
COPY . .
RUN make build