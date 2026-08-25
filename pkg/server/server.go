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
	cacheRules      []CacheRule
	redirects       map[string]string
	redirectsSource string
	webroot         *fs.FS
	metrics         *metrics
	registry        *prometheus.Registry
	staticFiles     *staticManifest
}

// DefaultCacheControl is the cache policy used when no configured rule matches.
const DefaultCacheControl = "public, max-age=31536000, must-revalidate"

// CacheRule associates a path pattern with a Cache-Control header value.
// Rules are evaluated in order against the resolved, webroot-relative file path,
// and the first matching rule wins. Patterns use path.Match syntax; patterns
// without a slash match a file's base name at any depth.
type CacheRule struct {
	Pattern      string
	CacheControl string
}

// ServerOption configures a Server.
type ServerOption func(*Server)

// WithCacheRules configures ordered, path-based cache rules.
func WithCacheRules(rules ...CacheRule) ServerOption {
	return func(s *Server) {
		s.cacheRules = append([]CacheRule(nil), rules...)
	}
}

// WithStaticFiles enables serving from an immutable manifest prepared with
// PrepareStaticFiles. The manifest must be prepared from the webroot passed to
// NewServer.
func WithStaticFiles(files *StaticFiles) ServerOption {
	return func(s *Server) {
		if files != nil {
			s.staticFiles = files.manifest
		}
	}
}

// NewServer returns a newly constructed Server
func NewServer(webroot *fs.FS, src string, opts ...ServerOption) *Server {
	reg := prometheus.NewRegistry()
	s := &Server{
		redirects:       map[string]string{},
		redirectsSource: src,
		webroot:         webroot,
		metrics:         newMetrics(reg),
		registry:        reg,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
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
