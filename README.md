[![Build Status](https://travis-ci.org/stripe/goforit.svg?branch=master)](https://travis-ci.org/stripe/goforit)
[![GoDoc](https://godoc.org/github.com/stripe/goforit?status.svg)](http://godoc.org/github.com/stripe/goforit)

goforit is an experimental, quick-and-dirty client library for feature flags in Go.

# Backends

Feature flags can be stored in any desired backend. goforit provides a flatfile implementation out-of-the-box, so feature flags can be defined in a [CSV][CSV].

Alternatively, flags can be stored in a key-value store like Consul or Redis.


# Usage

Create a CSV that defines the flag names and sampling rates:

```csv
go.sun.money,0
go.moon.mercury,1
go.stars.money,.5
```

```go
func main {
	backend := BackendFromFile("flags.csv")
	Init(30*time.Second, backend)

	if Enabled("go.sun.mercury") {
		fmt.Println("The go.sun.mercury feature is enabled for 100% of requests")
	}

	if Enabled("go.stars.money") {
		fmt.Println("The go.stars.money feature is enabled for 50% of requests")
	}
}
```


# Status

goforit is in an experimental state and may introduce breaking changes without notice.

[CSV]: https://github.com/stripe/goforit/blob/master/fixtures/flags_example.csv
