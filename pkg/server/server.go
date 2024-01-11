package server

import (
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/jnsgruk/gosherve/pkg/logging"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Server is responsible for the management of a Gosherve instance.
// This includes the logger, metrics, configuration and starting the
// HTTP server.
type Server struct {
	redirects       map[string]string
	redirectsSource string
	webroot         *fs.FS
	metrics         *metrics
	registry        *prometheus.Registry
}

// NewServer returns a newly constructed Server
func NewServer(webroot *fs.FS, src string) *Server {
	reg := prometheus.NewRegistry()
	return &Server{
		redirects:       map[string]string{},
		redirectsSource: src,
		webroot:         webroot,
		metrics:         newMetrics(reg),
		registry:        reg,
	}
}

// Start is used to start the Gosherve server, listening on port 8080.
// A metrics server is also started on port 8081.
func (s *Server) Start() {
	// Run the metrics handler on a separate HTTP server and different port
	go func() {
		http.Handle("/metrics", promhttp.HandlerFor(s.registry, promhttp.HandlerOpts{}))
		slog.Info("starting metrics server", "port", 8081)
		http.ListenAndServe(":8081", nil)
	}()

	r := http.NewServeMux()
	r.HandleFunc("/", s.routeHandler)
	slog.Info("starting gosherve server", "port", 8080)
	http.ListenAndServe(":8080", logging.RequestLoggerMiddleware(r))
}
