package goforit_test

import (
	"context"
	"fmt"
	"time"

	"github.com/stripe/goforit"
)

func Example() {
	ctx := context.Background()

	// flags.csv contains comma-separated flag names and sample rates.
	// See: fixtures/flags_example.csv
	backend := goforit.BackendFromFile("flags.csv")
	flags := goforit.New(30*time.Second, backend)

	if flags.Enabled(ctx, "go.sun.mercury", nil) {
		fmt.Println("The go.sun.mercury feature is enabled for 100% of requests")
	}
	// Same thing.
	if flags.Enabled(nil, "go.sun.mercury", nil) {
		fmt.Println("The go.sun.mercury feature is enabled for 100% of requests")
	}

	if flags.Enabled(ctx, "go.stars.money", nil) {
		fmt.Println("The go.stars.money feature is enabled for 50% of requests")
	}
}
