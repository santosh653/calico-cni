FROM golang:1.6-alpine

MAINTAINER Tom Denham <tom@tigera.io>

RUN apk -U add bash git musl-dev gcc linux-headers iproute2
WORKDIR /go/src/github.com/projectcalico/calico-cni
RUN go get github.com/stretchr/testify
RUN go get github.com/dchest/uniuri
RUN go get github.com/onsi/gomega
RUN apk add --update-cache --repository http://dl-cdn.alpinelinux.org/alpine/edge/testing etcd
RUN apk add curl && curl -o glibc.apk -L "https://github.com/andyshinn/alpine-pkg-glibc/releases/download/2.23-r1/glibc-2.23-r1.apk" && \
  apk add --allow-untrusted glibc.apk && \
  curl -o glibc-bin.apk -L "https://github.com/andyshinn/alpine-pkg-glibc/releases/download/2.23-r1/glibc-bin-2.23-r1.apk" && \
  apk add --allow-untrusted glibc-bin.apk && \
  /usr/glibc-compat/sbin/ldconfig /lib /usr/glibc/usr/lib && \
  echo 'hosts: files mdns4_minimal [NOTFOUND=return] dns mdns4' >> /etc/nsswitch.conf && \
  rm -f glibc.apk glibc-bin.apk