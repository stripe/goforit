package goforit

type noopMetricsClient struct{}

func (n noopMetricsClient) Histogram(s string, f float64, strings []string, f2 float64) error {
	return nil
}

func (n noopMetricsClient) TimeInMilliseconds(name string, milli float64, tags []string, rate float64) error {
	return nil
}

func (n noopMetricsClient) Gauge(s string, f float64, strings []string, f2 float64) error {
	return nil
}

func (n noopMetricsClient) Count(s string, i int64, strings []string, f float64) error {
	return nil
}

func (n noopMetricsClient) Close() error {
	return nil
}

var _ MetricsClient = noopMetricsClient{}
