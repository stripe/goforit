package goforit

import (
	"context"
	"time"
)

var globalGoforit *goforit

func init() {
	globalGoforit = New()
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
