package server

import (
	"net/http"

	"github.com/jnsgruk/gosherve/internal/logging"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Server is responsible for the management of a Gosherve instance.
// This includes the logger, metrics, configuration and starting the
// HTTP server.
type Server struct {
	Redirects map[string]string

	webroot         string
	redirectsSource string
	reg             *prometheus.Registry

	requestsTotal   prometheus.Counter
	redirectsServed *prometheus.CounterVec
	redirectsTotal  prometheus.Gauge
}

// NewServer returns a newly constructed Server
func NewServer(webroot string, src string, reg *prometheus.Registry) *Server {
	m := &Server{
		webroot:         webroot,
		redirectsSource: src,
		reg:             reg,

		requestsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "gosherve",
			Name:      "requests_total",
			Help:      "The total number of HTTP requests made to Gosherve.",
		}),
		redirectsServed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "gosherve",
			Name:      "redirects_served",
			Help:      "The number of requests per redirect",
		}, []string{"alias"}),
		redirectsTotal: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "gosherve",
			Name:      "redirects_total",
			Help:      "The number of redirects defined",
		}),
	}
	reg.MustRegister(m.requestsTotal, m.redirectsServed, m.redirectsTotal)
	return m
}

// Start is used to start the Gosherve server, listening on the configured
// port for HTTP requests.
func (s *Server) Start() {
	r := http.NewServeMux()
	r.Handle("/", RouteHandler{manager: s})
	r.Handle("/metrics", promhttp.HandlerFor(s.reg, promhttp.HandlerOpts{}))
	http.ListenAndServe(":8080", logging.RequestLoggerMiddleware(r))
}
