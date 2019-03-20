# Rules-based flags

This is a proposal for feature flags that exhibit rich behavior for determining if a flag is on or off. The core ideas are:

* When a flag is checked, a set of properties or tags are specified
* A flag's current settings are specified with a list of rules or conditions, which are executed in sequence to calculate whether or not the flag is enabled

## Using these flags

When a client program wants to check if a flag is enabled, it specifies a set of properties. Properties are just a mapping of string keys to string values:

```go
if goforit.Enabled(ctx, "myflag", map[string]string{"user": "bob"}) {
  ...
}
```

Some properties should never change during a program's execution, such as "hostname", "cluster" or "service". These can be preset when the program starts:

```go
goforit.AddDefaultTags(map[string]string{
  "hostname": "myhost.com",
  "service": "myprogram",
})
```

When `.Enabled()` is called, the explicit properties are merged with the default properties—if any properties are in both, the explicit ones take precedence.


## Determining if a flag is enabled

A flag's settings include the following:

* A boolean to determine whether the flag is "active". This can be used to disable a flag while leaving its rules otherwise intact.
* A list of zero or more rules, each of which has:
	* A type
	* Two actions to be performed if the rule matches, or if it doesn't match (aka "misses"). These can be either "on", "off", or "continue"
	* Some rule-type-specific properties

To determine if a flag is enabled, first the following special cases are considered:

* If the flag is not active, the flag is considered disabled
* If the flag is active but has no rules, the flag is considered enabled

Otherwise, the rules are executed in sequence. Each rule yields either a "match" or a "miss". (If any rule yields an error, that is considered a "miss", and the error is logged.) Depending on that result, the appropriate action is executed:

* If the action is "on", the flag is considered enabled. Further rules are skipped
* If the action is "off", the flag is considered disabled. Further rules are skipped
* If the action is "continue", the next rule is executed. If this is the last rule for this flag, the flag is considered disabled



## Rule types

The following rule types are currently defined:

### match_list

This rule type matches a property against a list of values. It has the following attributes:

* property: The name of the property to match
* values: A list of values to be matched against

Eg, given the following settings:

```
{
  "property": "user",
  "values: ["alice", "bob"]
}
```

The following usage would match this rule:

```go
goforit.Enabled(ctx, "myflag", map[string]string{"user": "bob"})
```

This usage would **not** match this rule:

```go
goforit.Enabled(ctx, "myflag", map[string]string{"user": "xavier"})
```

If the "user" property was not provided at all, that would be an error.


### sample

This rule type matches a given fraction of the time. It has the following attributes:

* properties: The names of the properties for sampling
* rate: The fraction of the time we should match, as a float from 0 to 1

This rule type effectively has two modes:

1. When 'properties' is empty, it just randomly matches at the given rate. Eg, this will match 5% of the time `.Enabled()` is called:

	```
	{
	  "properties": [],
	  "rate": 0.05
	}
	```

2. When 'properties' is non-empty, it deterministically matches the given fraction of values for those properties. Eg:

	```
	{
	  "properties": ["user", "currency"],
	  "rate": 0.05
	}
	```

	This would match 5% of (user, currency) value pairs. Each such pair would either always match or always not-match.

	If the caller to `.Enabled()` does not provide any of the given properties, it is an error.


## JSON file format

A JSON file is used to specify the current settings for each flag. The overall file format is:

```
{
  "updated": 1234567.89,
  "flags": [
    // A list of flag objects
  ]
}
```

Each flag object looks like the following:

```
{
  "name": "myflag",
  "active": true,
  "rules": [
    // A list of rule objects
  ]
}
```

Each rule has the basic format:

```
{
  "type": "sample",
  "on_match": "on",  // or "off", or "continue"
  "on_miss": "off",   // or "on", or "continue"

  // extra attributes particular to this type, eg:
  "rate": 0.5
}
```

That's it! Here's a complete but small example:

```
{
  "flags": [
    {
      "name": "go.sun.moon",
      "active": true,
      "rules": [
        {
          "type": "match_list",
          "property": "host_name",
          "values": [
            "srv_123",
            "srv_456"
          ],
          "on_match": "off",
          "on_miss": "continue"
        },
        {
          "type": "match_list",
          "property": "host_name",
          "values": [
            "srv_789"
          ],
          "on_match": "on",
          "on_miss": "continue"
        },
        {
          "type": "sample",
          "rate": 0.01,
          "properties": ["cluster", "db"],
          "on_match": "on",
          "on_miss": "off"
        }
      ]
    },
  ],
  "updated": 1519247256.0626957
}
```

## Examples

Here are some common use cases, and how to implement them with rules:

### Always off

```
{
	"name": "test.off",
	"active": false,
	"rules": []
}
```

### Always on

```
{
	"name": "test.on",
	"active": true,
	"rules": [
		{
			"type": "sample",
			"properties": [],
			"rate": 1,
			"on_match": "on",
			"on_miss": "off"
		}
	]
}
```

### Simple sampling

On 1% of the time:

```
{
	"name": "test.random",
	"active": true,
	"rules": [
		{
			"type": "sample",
			"properties": [],
			"rate": 0.01,
			"on_match": "on",
			"on_miss": "off"
		}
	]
}
```

### Sampling by property

On for 1% of users. Consistently on for the same set of users.

```
{
	"name": "test.random_by",
	"active": true,
	"rules": [
		{
			"type": "sample",
			"properties": ["user"],
			"rate": 0.01,
			"on_match": "on",
			"on_miss": "off"
		}
	]
}
```

### Allowlist

On for only Alice and Bob:

```
{
	"name": "test.allowlist",
	"active": true,
	"rules": [
		{
			"type": "match_list",
			"property": "user",
			"values": ["alice", "bob"],
			"on_match": "on",
			"on_miss": "off"
		}
	]
}
```

### Denylist

On for everyone except Xavier:

```
{
	"name": "test.denylist",
	"active": true,
	"rules": [
		{
			"type": "match_list",
			"property": "user",
			"values": ["xavier"],
			"on_match": "off",
			"on_miss": "on"
		}
	]
}
```

### Allowlist some, sample from the rest

This is useful for a new feature, with some explicit test users, and a random selection of other users:


```
{
	"name": "test.random_by_with_allowlist",
	"active": true,
	"rules": [
		{
			"type": "match_list",
			"property": "user",
			"values": ["test_user1", "test_user2"],
			"on_match": "on",
			"on_miss": "continue"
		},
		{
			"type": "sample",
			"properties": ["user"],
			"rate": 0.01,
			"on_match": "on",
			"on_miss": "off"
		}
	]
}
```

### Sample from only certain users

Only Alice should have this feature, and she should only see it for 5% of requests:


```
{
	"name": "test.allowlist_then_sample",
	"active": true,
	"rules": [
		{
			"type": "match_list",
			"property": "user",
			"values": ["alice"],
			"on_match": "continue",
			"on_miss": "off"
		},
		{
			"type": "sample",
			"properties": ["request_id"],
			"rate": 0.01,
			"on_match": "on",
			"on_miss": "off"
		}
	]
}
```

### Multiple allowlist

Multiple properties must all be matched:

```
{
	"name": "test.multi_allowlist",
	"active": true,
	"rules": [
		{
			"type": "match_list",
			"property": "user",
			"values": ["alice", "bob"],
			"on_match": "continue",
			"on_miss": "off"
		},
		{
			"type": "match_list",
			"property": "currency",
			"values": ["usd", "cad"],
			"on_match": "on",
			"on_miss": "off"
		}
	]
}
`
