FROM golang:1.10
MAINTAINER The Stripe Observability Team <support@stripe.com>

WORKDIR /go/src/github.com/stripe/goforit
ADD . /go/src/github.com/stripe/goforit
CMD go test -v -bench Bench ./...
