package goforit_test

import (
	"fmt"

	"github.com/stripe/goforit"
)

func Example() {
	// flags.csv contains comma-separated flag names and sample rates.
	// See: fixtures/flags_example.csv
	backend := goforit.NewCsvBackend("flags.csv", goforit.DefaultRefreshInterval)
	goforit.Init(backend)
	defer goforit.Close()

	if goforit.Enabled("go.sun.mercury", "tag", "value") {
		fmt.Println("The go.sun.mercury feature is enabled")
	}

	if goforit.Enabled("go.sun.mercury") {
		fmt.Println("The go.sun.mercury feature is enabled")
	}
}
