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

	enabled, err := Enabled(ctx, "go.sun.moon", map[string]string{"host_name": "apibox_123"})
	if err == nil && enabled {
		fmt.Println("The go.sun.mercury feature is enabled for 100% of requests")
	}
	// Same thing.
	enabled, err = Enabled(nil, "go.sun.mercury", nil)
	if err == nil && enabled {
		fmt.Println("The go.sun.mercury feature is enabled for 100% of requests")
	}

	enabled, err = Enabled(ctx, "go.stars.money", nil)
	if err == nil && enabled {
		fmt.Println("The go.stars.money feature is enabled for 50% of requests")
	}
}
