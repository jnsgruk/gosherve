package manager

import (
	"log/slog"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/jnsgruk/gosherve/internal/logging"
)

// RouteHandler is the route handler for all URL paths except
// metrics in Gosherve
type RouteHandler struct {
	manager *GosherveManager
}

// ServeHTTP serves the correct response based on the requested URL
// and updates metrics accordingly.
func (rh RouteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rh.manager.Metrics.RequestsTotal.Add(float64(1))
	l := logging.GetLoggerFromCtx(r.Context())

	if rh.manager.webroot == "" {
		handleRedirect(w, r, rh)
		return
	}

	var f string
	if r.URL.Path == "/" {
		f = path.Join(rh.manager.webroot, "index.html")
	} else {
		f = path.Join(rh.manager.webroot, r.URL.Path)
	}

	// Check if the requested file is in the defined webroot
	if _, err := os.Stat(f); os.IsNotExist(err) {
		// If the URL doesn't represent a valid file, treat request as a redirect
		handleRedirect(w, r, rh)
	} else {
		http.ServeFile(w, r, f)
		l.Info("served file", slog.Group("response", "status_code", 200, "file", f))
	}
}

// handleRedirect tries to lookup a redirect by its alias, returning the HTTP 301
// response if found.
func handleRedirect(w http.ResponseWriter, r *http.Request, rh RouteHandler) {
	l := logging.GetLoggerFromCtx(r.Context())

	alias := strings.TrimPrefix(r.URL.Path, "/")

	url, err := rh.manager.LookupRedirect(alias)
	if err != nil {
		handleNotFound(w, r, rh)
		return
	}

	rh.manager.Metrics.RedirectsServed.WithLabelValues(alias).Add(float64(1))

	rg := slog.Group("response", "location", url, "status_code", http.StatusMovedPermanently)
	l.Info("served redirect", rg)

	w.Header().Set("Access-Control-Allow-Origin", "*")
	http.Redirect(w, r, url, http.StatusMovedPermanently)
}

// handleNotFound handles invalid paths/redirects and returns a 404.html or plaintext "Not found"
func handleNotFound(w http.ResponseWriter, r *http.Request, rh RouteHandler) {
	l := logging.GetLoggerFromCtx(r.Context())
	logPlainResponse := func() {
		l.Error("not found", slog.Group("response", "status_code", 404, "text", "Not found"))
	}

	w.WriteHeader(http.StatusNotFound)

	if rh.manager.webroot == "" {
		w.Write([]byte("Not found"))
		logPlainResponse()
		return
	}

	// Check if there is a 404.html to return, otherwise return plaintext
	notFoundPagePath := path.Join(rh.manager.webroot, "404.html")
	content, err := os.ReadFile(notFoundPagePath)
	if err != nil {
		w.Write([]byte("Not found"))
		logPlainResponse()
		return
	}

	w.Write(content)
	l.Error("not found", slog.Group("response", "status_code", 404, "file", notFoundPagePath))
}
