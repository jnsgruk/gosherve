package manager

import (
	"log/slog"
	"net/http"

	"github.com/jnsgruk/gosherve/internal/logging"
	"github.com/jnsgruk/gosherve/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
)

// GosherveManager is responsible for the management of a Gosherve instance.
// This includes the logger, metrics, configuration and starting the
// HTTP server.
type GosherveManager struct {
	Redirects map[string]string
	Logger    *slog.Logger
	Metrics   *metrics.GosherveMetrics

	webroot         string
	redirectsSource string
	promRegistry    *prometheus.Registry
}

// NewGosherveManager returns a newly constructed GosherveManager
func NewGosherveManager(logger *slog.Logger) *GosherveManager {
	reg := prometheus.NewRegistry()
	m := metrics.NewGosherveMetrics(reg)

	return &GosherveManager{
		Logger:  logger,
		Metrics: m,

		webroot:         viper.GetString("webroot"),
		redirectsSource: viper.GetString("redirect_map_url"),
		promRegistry:    reg,
	}
}

// Start is used to start the Gosherve instance, listening on the configured
// port for HTTP requests.
func (gm *GosherveManager) Start() {
	r := http.NewServeMux()
	r.Handle("/", RouteHandler{manager: gm})
	r.Handle("/metrics", promhttp.HandlerFor(gm.promRegistry, promhttp.HandlerOpts{}))
	http.ListenAndServe(":8080", logging.RequestLoggerMiddleware(r, gm.Logger))
}
