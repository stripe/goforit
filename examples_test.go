package goforit

import (
	"context"
	"fmt"
	"time"
)

func Example() {
	ctx := context.Background()

	// flags.csv contains comma-separated flag names and sample rates.
	// See: fixtures/flags_example.csv
	backend := BackendFromFile("flags.csv")
	Init(30*time.Second, backend)

	if Enabled(ctx, "go.sun.mercury",  map[string]string{"host_name": "apibox_789"}) {
		fmt.Println("The go.sun.mercury feature is enabled for 100% of requests")
	}
	// Same thing.
	if Enabled(nil, "go.sun.mercury", nil) {
		fmt.Println("The go.sun.mercury feature is enabled for 100% of requests")
	}

	if Enabled(ctx, "go.stars.money", nil) {
		fmt.Println("The go.stars.money feature is enabled for 50% of requests")
	}
}
