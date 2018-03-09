package goforit

import (
	"context"
	"time"

	"github.com/getsentry/raven-go"
)

var globalGoforit *goforit

func init() {
	globalGoforit = newWithoutInit()
}

func Enabled(ctx context.Context, name string, props map[string]string) (enabled bool) {
	return globalGoforit.Enabled(ctx, name, props)
}

func RefreshFlags(backend Backend) {
	globalGoforit.RefreshFlags(backend)
}

func SetStalenessThreshold(threshold time.Duration) {
	globalGoforit.SetStalenessThreshold(threshold)
}

func AddDefaultTags(tags map[string]string) {
	globalGoforit.AddDefaultTags(tags)
}

func Init(interval time.Duration, backend Backend) {
	globalGoforit.init(interval, backend)
}

func Close() error {
	return globalGoforit.Close()
}

func SetupSentry(r *raven.Client) {
	globalGoforit.SetupSentry(r)
}
