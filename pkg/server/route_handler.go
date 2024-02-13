package server

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jnsgruk/gosherve/pkg/logging"
)

// routeHandler handles all routes for the Gosherve application except /metrics
func (s *Server) routeHandler(w http.ResponseWriter, r *http.Request) {
	s.metrics.requestsTotal.Inc()
	l := logging.GetLoggerFromCtx(r.Context())

	// First check if there is a redirect defined, and serve it if there is.
	if redirected := handleRedirect(w, r, s); redirected {
		return
	}

	// If there is no webroot, and the redirect isn't defined, then 404.
	if s.webroot == nil {
		handleNotFound(w, r, s)
		return
	}

	var filepath string
	if r.URL.Path == "/" {
		// Serve "index.html" if someone browses to the root
		filepath = "index.html"
	} else {
		filepath = strings.TrimPrefix(r.URL.Path, "/")
		filepath = strings.TrimSuffix(filepath, "/")
	}

	// Stat the file and report not found if that fails
	fi, err := fs.Stat(*s.webroot, filepath)
	if err != nil {
		handleNotFound(w, r, s)
		return
	}

	// If the file is a directory, append "index.html" to the end
	if fi.IsDir() {
		filepath = fmt.Sprintf("%s/index.html", filepath)
	}

	// First try reading the a file at the exact path sent
	b, err := fs.ReadFile(*s.webroot, filepath)
	if err != nil {
		handleNotFound(w, r, s)
		return
	}

	w.Header().Set("Cache-Control", "public, max-age=31536000, must-revalidate")
	w.Header().Set("ETag", fmt.Sprintf(`"%d-%x"`, len(b), sha1.Sum(b)))

	http.ServeContent(w, r, filepath, time.Now(), bytes.NewReader(b))
	s.metrics.responseStatus.WithLabelValues(strconv.Itoa(http.StatusOK)).Inc()
	l.Info("served file", slog.Group("response", "status_code", http.StatusOK, "file", filepath))
}

// handleRedirect tries to lookup a redirect by its alias, returning the HTTP 301
// response if found.
func handleRedirect(w http.ResponseWriter, r *http.Request, s *Server) bool {
	l := logging.GetLoggerFromCtx(r.Context())

	alias := strings.TrimPrefix(r.URL.Path, "/")

	url, err := s.LookupRedirect(alias)
	if err != nil {
		return false
	}

	s.metrics.redirectsServed.WithLabelValues(alias).Inc()
	s.metrics.responseStatus.WithLabelValues(strconv.Itoa(http.StatusMovedPermanently)).Inc()

	rg := slog.Group("response", "location", url, "status_code", http.StatusMovedPermanently)
	l.Info("served redirect", rg)

	w.Header().Set("Access-Control-Allow-Origin", "*")
	http.Redirect(w, r, url, http.StatusMovedPermanently)

	return true
}

// handleNotFound handles invalid paths/redirects and returns a 404.html or plaintext "Not found"
func handleNotFound(w http.ResponseWriter, r *http.Request, s *Server) {
	l := logging.GetLoggerFromCtx(r.Context())
	s.metrics.responseStatus.WithLabelValues(strconv.Itoa(http.StatusNotFound)).Inc()

	plainNotFound := func() {
		http.Error(w, "Not found", http.StatusNotFound)
		l.Error("not found", slog.Group("response", "status_code", http.StatusNotFound, "text", "Not found"))
	}

	if s.webroot == nil {
		plainNotFound()
		return
	}

	// Check if there is a 404.html to return, otherwise return plaintext
	content, err := fs.ReadFile(*s.webroot, "404.html")
	if err != nil {
		plainNotFound()
		return
	}

	http.Error(w, string(content), http.StatusNotFound)
	l.Error("not found", slog.Group("response", "status_code", http.StatusNotFound, "file", "404.html"))
}
