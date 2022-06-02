[![Build Status](https://travis-ci.org/stripe/goforit.svg?branch=master)](https://travis-ci.org/stripe/goforit)
[![GoDoc](https://godoc.org/github.com/stripe/goforit?status.svg)](http://godoc.org/github.com/stripe/goforit)

goforit is an experimental, quick-and-dirty client library for feature flags in Go.

# Backends

Feature flags can be stored in any desired backend. goforit provides a several flatfile implementations out-of-the-box, so feature flags can be defined in a JSON or CSV file. See below for details.    

Alternatively, flags can be stored in a key-value store like Consul or Redis.


# Usage

Create a CSV file that defines the flag names and sampling rates:

```csv
go.sun.money,0
go.moon.mercury,1
go.stars.money,.5
```

```go
func main() {
	ctx := context.Background()

	// flags.csv contains comma-separated flag names and sample rates.
	// See: testdata/flags_example.csv
	backend := goforit.BackendFromFile("flags.csv")
	goforit.Init(30*time.Second, backend)

	if goforit.Enabled(ctx, "go.sun.mercury", nil) {
		fmt.Println("The go.sun.mercury feature is enabled for 100% of requests")
	}

	if goforit.Enabled(ctx, "go.stars.money", nil) {
		fmt.Println("The go.stars.money feature is enabled for 50% of requests")
	}
}
```

# Backends

Included flatfile backends are:

## CSV

This is a very simple backend, where every row defines a flag name and a rate at which it should be enabled, between zero and one. Initialize this backend with `BackendFromFile`. See [an example][CSV].

## JSON v1

This backend allows each flag to have multiple rules, like a series of if-statements. Each call to `.Enabled()` takes a map of properties, which rules can match against. Each rule's matching or non-matching can cause the overall flag to be on or off, or can fallthrough to the next rule. See [the proposal for this system][JSON1_proposal] or [an example JSON file][JSON1]. It's a bit confusing to understand.

## JSON v2

In this format, each flag can have a number of rules, and each rule can contain a number of predicates for matching properties. When a flag is evaluated, it uses the first rule whose predicates match the given properties. See [an example JSON file, that also includes test cases][JSON2].

# Status

goforit is in an experimental state and may introduce breaking changes without notice.

[CSV]: https://github.com/stripe/goforit/blob/master/testdata/flags_example.csv
[JSON1_proposal]: https://github.com/stripe/goforit/blob/master/doc/rule_flags.md
[JSON1]: https://github.com/stripe/goforit/blob/master/testdata/flags_example.json
[JSON2]: https://github.com/stripe/goforit/blob/master/testdata/flags2_acceptance.json

# License

goforit is available under the MIT license.
