FROM golang:1.13.5
MAINTAINER The Stripe Observability Team <support@stripe.com>

RUN mkdir -p /build
ENV GOPATH=/go

RUN go get -u -v github.com/kardianos/govendor

WORKDIR /go/src/github.com/stripe/goforit
ADD . /go/src/github.com/stripe/goforit


CMD whoami
