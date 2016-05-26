FROM golang:1.6-alpine

MAINTAINER Tom Denham <tom@tigera.io>

RUN apk -U add bash git musl-dev gcc linux-headers iproute2
WORKDIR /go/src/github.com/projectcalico/calico-cni
RUN go get github.com/stretchr/testify
RUN go get github.com/dchest/uniuri

