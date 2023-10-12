package manager

import (
	"net/http"

	"github.com/jnsgruk/gosherve/internal/logging"
	"github.com/jnsgruk/gosherve/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// GosherveManager is responsible for the management of a Gosherve instance.
// This includes the logger, metrics, configuration and starting the
// HTTP server.
type GosherveManager struct {
	Redirects map[string]string
	Metrics   *metrics.GosherveMetrics

	webroot         string
	redirectsSource string
	promRegistry    *prometheus.Registry
}

// NewGosherveManager returns a newly constructed GosherveManager
func NewGosherveManager(webroot string, src string) *GosherveManager {
	reg := prometheus.NewRegistry()
	m := metrics.NewGosherveMetrics(reg)

	return &GosherveManager{
		Metrics: m,

		webroot:         webroot,
		redirectsSource: src,
		promRegistry:    reg,
	}
}

// Start is used to start the Gosherve instance, listening on the configured
// port for HTTP requests.
func (gm *GosherveManager) Start() {
	r := http.NewServeMux()
	r.Handle("/", RouteHandler{manager: gm})
	r.Handle("/metrics", promhttp.HandlerFor(gm.promRegistry, promhttp.HandlerOpts{}))
	http.ListenAndServe(":8080", logging.RequestLoggerMiddleware(r))
}
