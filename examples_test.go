package goforit

import "fmt"

func Example() {
	// flags.csv contains comma-separated flag names and sample rates.
	// See: fixtures/flags_example.csv
	backend := NewCsvBackend("flags.csv", DefaultRefreshInterval)
	Init(backend)
	defer Close()

	if Enabled("go.sun.mercury", map[string]string{"tag": "value"}) {
		fmt.Println("The go.sun.mercury feature is enabled")
	}

	if Enabled("go.sun.mercury", nil) {
		fmt.Println("The go.sun.mercury feature is enabled")
	}
}
