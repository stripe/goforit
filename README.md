<!--[![Build Status](https://travis-ci.org/stripe/goforit.svg?branch=master)](https://travis-ci.org/stripe/goforit)
[![GoDoc](https://godoc.org/github.com/stripe/goforit?status.svg)](http://godoc.org/github.com/stripe/goforit)
-->
goforit is an experimental, client library for feature flags in Go.

# Backends

Feature flags can be stored in any desired backend. goforit provides a flatfile implementation out-of-the-box, so feature flags can be defined in a [CSV][CSV] file, as well as a more complex [JSON][JSON]-based format.

Alternatively, flags could theoretically be stored in a key-value store like Consul or Redis.

# Usage

Create a CSV file that defines the flag names and sampling rates:

```csv
go.sun.money,0
go.moon.mercury,1
go.stars.money,.5
```

```go
func main() {
	// flags.csv contains comma-separated flag names and sample rates.
	// See: fixtures/flags_example.csv
	backend := goforit.NewCsvBackend("flags.csv", goforit.DefaultRefreshInterval)
	goforit.Init(backend)

	if goforit.Enabled("go.sun.mercury") {
		fmt.Println("The go.sun.mercury feature is enabled for 100% of requests")
	}

	if goforit.Enabled("go.stars.money") {
		fmt.Println("The go.stars.money feature is enabled for 50% of requests")
	}
}
```

## Tags

Feature can be more complex than just a rate! For example, you might want a flag to be on only for certain users. To allow this, you can tags when checking whether or not a flag is on:

```go
tags := map[string]string{"user": "alice", "request_id": "12345"}
if goforit.Enabled("my.new.feature", tags) {
	doSomething()
}

```

As a convenient shortcut, you can also specify flags as key and value arguments:

```go
if goforit.Enabled("my.new.feature", "user", "alice", "request_id", "12345") {
	doSomething()
}
```

Finally, some tags usually don't change for the life of your program. You can set these up globally:

```go
goforit.AddDefaultTags(map[string]string{
	"server": "myhost.mydomain.com",
	"cluster": "green",
})

// The "server" and "cluster" tags are automatically merged with "user".
if goforit.Enabled("my.new.feature", "user", "alice") {
	doSomething()
} 
```

## Options

When initializing goforit, you can provide a variety of options, eg:

```go
logger := log.New(os.Stderr, "mylogger ", 0)
defaultTags := map[string]string{"server", "myserver"}

goforit.Init(backend,
	LogErrors(logger),
	Tags(defaultTags),
)
```

Options include:

* LogErrors: Log errors to a logger
* MaxStaleness: If the current flag definition becomes older than a certain duration, yield an error (see below)
* OverrideFlag: Override flag statuses for testing (see below)
* Seed: Set the random number generator seed, for reproducible results
* SuppressErrors: Don't log errors at all. By default, they're logged to stderr
* Tags: Set the default tags
* OnError, OnAge, OnCheck: Callbacks, see below

You also don't have to use a global goforit instance—but can set one up that's independent. This could be useful in large programs where different components each have their own feature flag system:

```go
flagset := goforit.New(backend, LogErrors(logger))

if flagset.Enabled("my.new.feature") {
	doSomething()
}
```

## Callbacks

In your code where you're calling `Enabled` is usually not the right place to handle errors and metrics internal to goforit! Instead, goforit provides a callback system:

```go
goforit.Init(backend,
   // Handle errors
   OnError(func(err error) { logMyError(err) }),
   
   // Get notified about the age of the current set of feature flags
   OnAge(func(ty goforit.AgeType, age time.Duration) { checkIfTooOld(age) }),
   
   // Get notified when someone called Enabled()
   OnCheck(func(flag string, enabled bool) { reportMetric(flag, enabled) }),
)
```

You can provide multiple of each type of callback, and all of them will be called.


Built-in integrations exist for Sentry and DataDog. The Sentry integration will report errors to Sentry, and the DataDog integration handles errors as well as age and check metrics.

```go
import (
	"github.com/stripe/goforit"
	"github.com/stripe/goforit/integrations/datadog"
	"github.com/stripe/goforit/integrations/sentry"
)

goforit.Init(backend,
    datadog.Statsd(statsdAddress),
    sentry.SentryDSN(dsn),
)
```

## Testing

When running tests, you want feature flags to be in a known state. To handle that, goforit allows overriding the status of a flag:

```go
func TestSomething(t *testing.T) {
	backend := NewEmptyBackend() // will always say flags are off
	flagset := goforit.New(backend)
	
	obj := myObj{flagset: flagset, ...}
	obj.doSomeWork()
	
	// Force a flag to be on
	flagset.Override("my.new.feature", true)
	
	obj.doSomeMoreWork()
}
```

## Turning goforit off

If you don't want to initialize goforit under some conditions, that's fine! Calling `Enabled()` will just always return false, and will log a message every hour to remind you that flags are not initialized.

If you don't even want the message, you can just initialize goforit with `NewEmptyBackend()`.


# Condition-list flags

In addition to the simple CSV format, goforit supports a more complex format usable with JSON files. This allows flags to have whitelists, blacklists and different kinds of sampling.

In this format, each flag is defined by a list of _conditions_. Each condition is matched against in turn, and then executes an _action_ depending on whether or not it matched.

## Conditions

Two kinds of condition are currently defined:

### in_list

This condition checks if a given tag is in a list. For example, the following will match if the _user_ tag is either _alice_ or _bob_:

```json
{
	"type": "in_list",
	"tag": "user",
	"values": ["alice", "bob"]
}
```

### sample

This condition checks if a certain probability is met. It has two use cases:

* In basic usage, it just checks a random number against a rate. The following will match 10% of the time:

		{
			"type": "sample",
			"rate": 0.1
		}


* In a more complex usage, you can provide a rate and a list of tag names. It will then match such that the same tag values always give the same results. For example:

		{
			"type": "sample",
			"rate": 0.1,
			"tags": ["user"]
		}
 
 	This will match 10% of users, such that if a user matches once, it will always match in the future. The matching is hash-based: non-random but not easily predictable.
 
## Actions

Each condition comes with two action properties:

* `on_match` executes if the condition matched
* `on_miss` executes if the condition did not match

The following are valid action values:

* `enabled` causes the current Enabled() call to immediately yield true
* `disabled` causes the current Enabled() call to immediately yield false
* `next` causes the next condition to run. If there is no next condition, this is equivalent to `disabled`

By default, on\_match is set to `enabled`, and on\_miss is set to `next`.

## Putting them together

By combining conditions and actions, you can achieve interesting results! For example, this is a blacklist—note how on_match is **disabled** instead of enabled.

```json
{
	"type": "in_list",
	"tag": "user",
	"values": ["alice", "bob"],
	"on_match": "disabled",
	"on_miss": "enabled"
}
```

## Building a flag

A flag is then just a name, and a list of conditions. Flags also have an `active` setting as a sort of circuit-breaker—if it's false, the flag is off irrespective of any conditions.

Here's an example flag that is on for 2% of users in Japan:

```json
{
  "name": "go.two_percent_of_japan",
  "active": true,
  "conditions": [
    {
      "type": "in_list",
      "tag": "country",
      "values": ["japan"],
      "on_match": "next",
      "on_miss": "disabled"
    },
    {
      "type": "sample",
      "tags": "user",
      "rate": 0.02,
    }
  ]
}
```

## Building a file

Finally, the global structure of the JSON file is as follows:

```json
{
	// The only version supported
	"version": 1,
		
	// Epoch time of last known update
	"updated": 1519247256.0626957,
		
	// A list of flags
	"flags": [
		{
			"name": "go.test",
			"active": true,
			// ...
		},
		// ...
	]
}
```


# Status

goforit is in an experimental state and may introduce breaking changes without notice.

[CSV]: https://github.com/stripe/goforit/blob/master/fixtures/flags_example.csv
[JSON]: https://github.com/stripe/goforit/blob/master/fixtures/flags_condition_example.json
