[![Build Status](https://travis-ci.org/stripe/goforit.svg?branch=master)](https://travis-ci.org/stripe/goforit)
[![GoDoc](https://godoc.org/github.com/stripe/goforit?status.svg)](http://godoc.org/github.com/stripe/goforit)

goforit is an experimental, quick-and-dirty client library for feature flags in Go.

# Backends

Feature flags can be stored in any desired backend. goforit provides a flatfile implementation out-of-the-box, so feature flags can be defined in a [CSV](https://github.com/stripe/goforit/blob/master/fixtures/flags_example.csv).

Alternatively, flags can be stored in a key-value store like Consul or Redis.


# Status

goforit is in an experimental state and may introduce breaking changes without notice.
