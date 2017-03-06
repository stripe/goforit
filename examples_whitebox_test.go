package goforit_test

import (
	"fmt"
	"time"

	"github.com/stripe/goforit"
)

func Example() {
	// flags.csv contains comma-separated flag names and sample rates.
	// See: fixtures/flags_example.csv
	backend := goforit.BackendFromFile("flags.csv")
	goforit.Init(30*time.Second, backend)

	if goforit.Enabled("go.sun.mercury") {
		fmt.Println("The go.sun.mercury feature is enabled for 100% of requests")
	}

	if goforit.Enabled("go.stars.money") {
		fmt.Println("The go.stars.money feature is enabled for 50% of requests")
	}
}
