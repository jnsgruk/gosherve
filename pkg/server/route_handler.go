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
	statusWriter := &statusResponseWriter{ResponseWriter: w}
	defer func() {
		s.metrics.responseStatus.WithLabelValues(strconv.Itoa(statusWriter.StatusCode())).Inc()
	}()

	if servedFile := handleFile(statusWriter, r, s); servedFile {
		return
	}

	// First check if there is a redirect defined, and serve it if there is.
	if redirected := handleRedirect(statusWriter, r, s); redirected {
		return
	}

	// No file or redirect matched, so return a not found.
	handleNotFound(statusWriter, r, s)
}

// handleFile tries to serve a file based on the path in the request, returning a bool if successful.
func handleFile(w http.ResponseWriter, r *http.Request, s *Server) bool {
	l := logging.GetLoggerFromCtx(r.Context())

	// If there is no webroot, and the redirect isn't defined, then 404.
	if s.webroot == nil {
		return false
	}

	var filepath string
	var preparedFile *staticFile
	if s.staticFiles != nil {
		var ok bool
		preparedFile, ok = s.staticFiles.resolve(r.URL.Path)
		if !ok {
			return false
		}
		filepath = preparedFile.path
	} else {
		var ok bool
		filepath, ok = resolveFilePath(r.URL.Path, s.webroot)
		if !ok {
			return false
		}
	}

	w.Header().Set("Cache-Control", s.cacheControl(filepath))
	if preparedFile != nil {
		if err := servePreparedFile(w, r, s.staticFiles, preparedFile); err != nil {
			if err == errNotAcceptable {
				http.Error(w, "no acceptable content encoding", http.StatusNotAcceptable)
				l.Info("served file", slog.Group("response", "status_code", responseStatusCode(w), "file", filepath))
				return true
			}
			l.Error("unable to serve file", "file", filepath, "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return true
		}
	} else {
		if etag := calculateETag(filepath, s.webroot); etag != "" {
			w.Header().Set("ETag", etag)
		}
		http.ServeFileFS(w, r, *s.webroot, filepath)
	}
	l.Info("served file", slog.Group("response", "status_code", responseStatusCode(w), "file", filepath))

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
	rg := slog.Group("response", "location", url, "status_code", http.StatusMovedPermanently)
	l.Info("served redirect", rg)

	w.Header().Set("Access-Control-Allow-Origin", "*")
	http.Redirect(w, r, url, http.StatusMovedPermanently)

	return true
}

// handleNotFound handles invalid paths/redirects and returns a 404.html or plaintext "Not found"
func handleNotFound(w http.ResponseWriter, r *http.Request, s *Server) {
	l := logging.GetLoggerFromCtx(r.Context())
	plainNotFound := func() {
		http.Error(w, "Not found", http.StatusNotFound)
		l.Error("not found", slog.Group("response", "status_code", http.StatusNotFound, "text", "Not found"))
	}

	if s.webroot == nil {
		plainNotFound()
		return
	}
	if s.staticFiles != nil {
		file, ok := s.staticFiles.files["404.html"]
		if !ok {
			plainNotFound()
			return
		}

		w.Header().Set("Cache-Control", s.cacheControl(file.path))
		if err := servePreparedError(w, r, s.staticFiles, file, http.StatusNotFound); err != nil {
			if err == errNotAcceptable {
				http.Error(w, "no acceptable content encoding", http.StatusNotAcceptable)
				return
			}
			l.Error("unable to serve not-found page", "file", file.path, "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		l.Error("not found", slog.Group("response", "status_code", responseStatusCode(w), "file", file.path))
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
