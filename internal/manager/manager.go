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

type GosherveManager struct {
	Redirects map[string]string
	Logger    *slog.Logger
	Metrics   *metrics.GosherveMetrics

	serveFiles      bool
	redirectsSource string
	promRegistry    *prometheus.Registry
}

func NewGosherveManager(logger *slog.Logger) *GosherveManager {
	reg := prometheus.NewRegistry()
	m := metrics.NewGosherveMetrics(reg)

	return &GosherveManager{
		Logger:  logger,
		Metrics: m,

		serveFiles:      viper.Get("webroot") != nil,
		redirectsSource: viper.Get("redirect_map_url").(string),
		promRegistry:    reg,
	}
}

func (gm *GosherveManager) Start() {
	r := http.NewServeMux()
	r.Handle("/", RouteHandler{manager: gm})
	r.Handle("/metrics", promhttp.HandlerFor(gm.promRegistry, promhttp.HandlerOpts{}))
	http.ListenAndServe(":8080", logging.RequestLoggerMiddleware(r, gm.Logger))
}
