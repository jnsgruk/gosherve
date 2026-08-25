package server

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/jnsgruk/gosherve/pkg/logging"
)

// routeHandler handles all routes for the Gosherve application except /metrics
func (s *Server) routeHandler(w http.ResponseWriter, r *http.Request) {
	s.metrics.requestsTotal.Inc()

	if servedFile := handleFile(w, r, s); servedFile {
		return
	}

	// First check if there is a redirect defined, and serve it if there is.
	if redirected := handleRedirect(w, r, s); redirected {
		return
	}

	// No file or redirect matched, so return a not found.
	handleNotFound(w, r, s)
}

// handleFile tries to serve a file based on the path in the request, returning a bool if successful.
func handleFile(w http.ResponseWriter, r *http.Request, s *Server) bool {
	l := logging.GetLoggerFromCtx(r.Context())

	// If there is no webroot, and the redirect isn't defined, then 404.
	if s.webroot == nil {
		return false
	}

	filepath, ok := resolveFilePath(r.URL.Path, s.webroot)
	if !ok {
		return false
	}

	w.Header().Set("Cache-Control", s.cacheControl(filepath))
	if etag := calculateETag(filepath, s.webroot); etag != "" {
		w.Header().Set("ETag", etag)
	}

	http.ServeFileFS(w, r, *s.webroot, filepath)
	s.metrics.responseStatus.WithLabelValues(strconv.Itoa(http.StatusOK)).Inc()
	l.Info("served file", slog.Group("response", "status_code", http.StatusOK, "file", filepath))

	return true
}

// resolveFilePath resolves a request path to a file in the webroot. Directory
// requests resolve to the index.html within that directory.
func resolveFilePath(requestPath string, fsys *fs.FS) (string, bool) {
	filepath := strings.Trim(requestPath, "/")
	if filepath == "" {
		filepath = "index.html"
	}

	fi, err := fs.Stat(*fsys, filepath)
	if err != nil {
		return "", false
	}

	if fi.IsDir() {
		filepath = path.Join(filepath, "index.html")
		fi, err = fs.Stat(*fsys, filepath)
		if err != nil || fi.IsDir() {
			return "", false
		}
	}

	return filepath, true
}

// cacheControl returns the policy for the first rule matching the resolved
// file path, or the default policy if no rule matches.
func (s *Server) cacheControl(filepath string) string {
	for _, rule := range s.cacheRules {
		candidate := filepath
		if !strings.Contains(rule.Pattern, "/") {
			candidate = path.Base(filepath)
		}

		matched, err := path.Match(rule.Pattern, candidate)
		if err == nil && matched {
			return rule.CacheControl
		}
	}

	return DefaultCacheControl
}

// handleRedirect tries to lookup a redirect by its alias, returning the HTTP 301
// response if found.
func handleRedirect(w http.ResponseWriter, r *http.Request, s *Server) bool {
	l := logging.GetLoggerFromCtx(r.Context())

	alias := strings.Trim(r.URL.Path, "/")

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

	w.Header().Set("Cache-Control", s.cacheControl("404.html"))
	w.Header().Set("ETag", contentETag(content))
	w.Header().Set("Content-Type", "text/html")

	w.WriteHeader(http.StatusNotFound)
	w.Write(content)

	l.Error("not found", slog.Group("response", "status_code", http.StatusNotFound, "file", "404.html"))
}

// calculateETag calculates the ETag for a file based on its content.
func calculateETag(filename string, fsys *fs.FS) string {
	content, err := fs.ReadFile(*fsys, filename)
	if err != nil {
		return ""
	}

	return contentETag(content)
}

func contentETag(content []byte) string {
	return fmt.Sprintf(`"%x"`, sha256.Sum256(content))
}
