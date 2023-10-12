package server

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type metrics struct {
	requestsTotal    prometheus.Counter
	redirectsServed  *prometheus.CounterVec
	redirectsDefined prometheus.Gauge
	responseStatus   *prometheus.CounterVec
}

func newMetrics() *metrics {
	return &metrics{
		requestsTotal: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "gosherve",
			Name:      "requests_total",
			Help:      "The total number of HTTP requests made to Gosherve.",
		}),
		redirectsServed: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: "gosherve",
			Name:      "redirects_served",
			Help:      "The number of requests per redirect",
		}, []string{"alias"}),
		redirectsDefined: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: "gosherve",
			Name:      "redirects_defined",
			Help:      "The number of redirects defined",
		}),
		responseStatus: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: "gosherve",
			Name:      "response_status",
			Help:      "The status codes of HTTP responses",
		}, []string{"status"}),
	}
}
