package goforit

import (
	"context"
	"time"
)

var globalGoforit *goforit

func init() {
	globalGoforit = newWithoutInit(enabledTickerInterval)
}

func Enabled(ctx context.Context, name string, props map[string]string) (enabled bool) {
	return globalGoforit.Enabled(ctx, name, props)
}

func RefreshFlags(backend Backend) {
	globalGoforit.RefreshFlags(backend)
}

func TryRefreshFlags(backend Backend) error {
	return globalGoforit.TryRefreshFlags(backend)
}

func SetStalenessThreshold(threshold time.Duration) {
	globalGoforit.SetStalenessThreshold(threshold)
}

func AddDefaultTags(tags map[string]string) {
	globalGoforit.AddDefaultTags(tags)
}

func Init(interval time.Duration, backend Backend, opts ...Option) {
	globalGoforit.init(interval, backend, opts...)
}

func Close() error {
	return globalGoforit.Close()
}
