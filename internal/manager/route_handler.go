package manager

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/jnsgruk/gosherve/internal/logging"
)

type RouteHandler struct {
	manager *GosherveManager
}

// routeHandler is the initial URL handler for all paths
func (rh RouteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rh.manager.Metrics.RequestsTotal.Add(float64(1))
	l := logging.GetLoggerFromCtx(r.Context())

	if !rh.manager.serveFiles {
		handleRedirect(w, r, rh)
		return
	}

	var path string
	if r.URL.Path == "/" {
		path = os.Getenv("GOSHERVE_WEBROOT") + "/index.html"
	} else {
		path = os.Getenv("GOSHERVE_WEBROOT") + r.URL.Path
	}

	// Check if the requested file is in the defined webroot
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// If the URL doesn't represent a valid file, treat request as a redirect
		handleRedirect(w, r, rh)
	} else {
		http.ServeFile(w, r, path)
		l.Info("served file", slog.Group("response", "status_code", 200, "file", path))
	}
}

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

	if !rh.manager.serveFiles {
		w.Write([]byte("Not found"))
		logPlainResponse()
		return
	}

	// Check if there is a 404.html to return, otherwise return plaintext
	notFoundPagePath := fmt.Sprintf("%s/%s", os.Getenv("GOSHERVE_WEBROOT"), "404.html")
	content, err := os.ReadFile(notFoundPagePath)
	if err != nil {
		w.Write([]byte("Not found"))
		logPlainResponse()
		return
	}

	w.Write(content)
	l.Error("not found", slog.Group("response", "status_code", 404, "file", notFoundPagePath))
}
