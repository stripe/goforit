package goforit

import (
	"fmt"
	"time"
)

func Example() {
	// flags.csv contains comma-separated flag names and sample rates.
	// See: fixtures/flags_example.csv
	backend := BackendFromFile("flags.csv")
	Init(30*time.Second, backend)

	if Enabled("go.sun.mercury") {
		fmt.Println("The go.sun.mercury feature is enabled for 100% of requests")
	}

	if Enabled("go.stars.money") {
		fmt.Println("The go.stars.money feature is enabled for 50% of requests")
	}
}
