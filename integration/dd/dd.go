package dd

import (
	"fmt"
	"log"
	"reflect"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/stripe/goforit"
)

const goforitService = "goforit.backend.update"

// An interface reflecting the parts of statsd that we need, so we can mock it
type StatsdClient interface {
	Histogram(string, float64, []string, float64) error
	Incr(string, []string, float64) error
	SimpleServiceCheck(string, statsd.ServiceCheckStatus) error
}

func Statsd(stats StatsdClient) goforit.Option {
	return func(fs *goforit.Flagset) {
		goforit.OnAge(func(ag goforit.AgeType, age time.Duration) {
			// Send a histogram of ages, so we can detect out-of-date flags
			metric := "goforit.age." + string(ag)
			rate := 0.1
			if ag == goforit.AgeBackend {
				// Enabled calls could happen a lot, sample more
				rate = 0.01
			}
			stats.Histogram(metric, age.Seconds(), nil, rate)

			// Backend updates (aka source age callbacks) should happen reasonably often, so
			// log a service check then.
			if ag == goforit.AgeSource {
				stats.SimpleServiceCheck(goforitService, statsd.Ok)
			}
		})(fs)

		// Send errors
		goforit.OnError(func(err error) {
			errType := reflect.TypeOf(err).String()
			stats.Incr("goforit.error", []string{"error:" + errType}, 1)

			// Some errors are likely to indicate we're down
			if _, ok := err.(goforit.CriticalError); ok {
				stats.SimpleServiceCheck(goforitService, statsd.Warn)
			}
		})(fs)

		// Send metrics on checks
		goforit.OnCheck(func(name string, enabled bool) {
			stats.Incr("goforit.check", []string{
				"flag:" + name,
				fmt.Sprintf("enabled:%v", enabled),
			}, 0.01)
		})(fs)
	}
}

// Statsd reports information about this Flagset to DataDog
func StatsdAddr(addr string) goforit.Option {
	stats, err := statsd.New(addr)
	if err != nil {
		log.Printf("goforit can't initialize statsd client: %s", err)
		return func(*goforit.Flagset) {}
	}
	return Statsd(stats)
}
