FROM golang:1.6-alpine

MAINTAINER Tom Denham <tom@tigera.io>

RUN apk -U add bash git musl-dev gcc linux-headers
WORKDIR /go/src/github.com/projectcalico/calico-cni

