package server

import (
	"net/http"

	"github.com/jnsgruk/gosherve/internal/logging"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Server is responsible for the management of a Gosherve instance.
// This includes the logger, metrics, configuration and starting the
// HTTP server.
type Server struct {
	redirects       map[string]string
	redirectsSource string
	webroot         string
	metrics         *metrics
}

// NewServer returns a newly constructed Server
func NewServer(webroot string, src string) *Server {
	return &Server{
		redirectsSource: src,
		webroot:         webroot,
		metrics:         newMetrics(),
	}
}

// Start is used to start the Gosherve server, listening on port 8080
func (s *Server) Start() {
	r := http.NewServeMux()
	r.HandleFunc("/", s.routeHandler)
	r.Handle("/metrics", promhttp.Handler())
	http.ListenAndServe(":8080", logging.RequestLoggerMiddleware(r))
}
