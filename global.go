package goforit

import (
	"context"
	"time"

	"github.com/DataDog/datadog-go/statsd"
)

var globalGoforit goforit

func initGlobal() {
	globalGoforit.stats, _ = statsd.New(statsdAddress)
	globalGoforit.flags = map[string]Flag{}
}

func init() {
	initGlobal()
}

func Enabled(ctx context.Context, name string) (enabled bool) {
	return globalGoforit.Enabled(ctx, name)
}

func RefreshFlags(backend Backend) error {
	return globalGoforit.RefreshFlags(backend)
}

func SetStalenessThreshold(threshold time.Duration) {
	globalGoforit.SetStalenessThreshold(threshold)
}

func Init(interval time.Duration, backend Backend) *time.Ticker {
	return globalGoforit.Init(interval, backend)
}
