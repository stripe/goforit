package sentry

import (
	"log"

	raven "github.com/getsentry/raven-go"
	"github.com/stripe/goforit"
)

func Sentry(client *raven.Client) goforit.Option {
	if client == nil {
		client = raven.DefaultClient
	}
	return goforit.OnError(func(err error) {
		client.CaptureError(err, nil)
	})
}

// Statsd reports information about this Flagset to DataDog
func SentryDSN(dsn string) goforit.Option {
	client, err := raven.New(dsn)
	if err != nil {
		log.Printf("goforit can't initialize Sentry client: %s", err)
		return func(*goforit.Flagset) {}
	}
	return Sentry(client)
}
