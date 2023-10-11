package metrics

import "github.com/prometheus/client_golang/prometheus"

// GosherveMetrics is a struct containing all of the metrics for gosherve.
type GosherveMetrics struct {
	RequestsTotal   prometheus.Counter
	RedirectsServed *prometheus.CounterVec
	RedirectsTotal  prometheus.Gauge
}

// NewGosherveMetrics returns a newly constructed GosherveMetrics instance.
func NewGosherveMetrics(reg prometheus.Registerer) *GosherveMetrics {
	m := &GosherveMetrics{
		RequestsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "gosherve",
			Name:      "requests_total",
			Help:      "The total number of HTTP requests made to Gosherve.",
		}),
		RedirectsServed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "gosherve",
			Name:      "redirects_served",
			Help:      "The number of requests per redirect",
		}, []string{"alias"}),
		RedirectsTotal: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "gosherve",
			Name:      "redirects_total",
			Help:      "The number of redirects defined",
		}),
	}
	reg.MustRegister(m.RedirectsTotal, m.RedirectsServed, m.RequestsTotal)
	return m
}
