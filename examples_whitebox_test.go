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
	// See: testdata/flags2_example.json
	backend := goforit.BackendFromJSONFile2("testdata/flags2_example.json")
	flags := goforit.New(30*time.Second, backend, goforit.WithOwnedStats(true))
	defer func() { _ = flags.Close() }()

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
