package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"log/slog"
)

var logger *slog.Logger
var redirects map[string]string
var serveFiles bool = true

func main() {
	// Setup logging
	logger = getRootLogger(strings.ToLower(os.Getenv("GOSHERVE_LOG_LEVEL")))

	// Check for redirect map URL, exit application if absent
	if _, set := os.LookupEnv("GOSHERVE_REDIRECT_MAP_URL"); !set {
		logger.Error("GOSHERVE_REDIRECT_MAP_URL environment variable not set")
		os.Exit(1)
	}

	// Check if gosherve is required to serve files from a directory.
	if _, set := os.LookupEnv("GOSHERVE_WEBROOT"); !set {
		logger.Warn("GOSHERVE_WEBROOT environment variable not set")
		serveFiles = false
	}

	// Hydrate the redirects map
	newRedirects, err := fetchRedirects()
	if err != nil {
		// Since this is the first hydration, exit if unable to fetch redirects.
		// At this point, without the redirects to begin with the server is
		// quite useless.
		logger.Error("error fetching redirect map")
		os.Exit(1)
	}
	redirects = newRedirects

	r := http.NewServeMux()
	r.HandleFunc("/", routeHandler)
	http.ListenAndServe(":8080", requestLogger(r))
}

// routeHandler is the initial URL handler for all paths
func routeHandler(w http.ResponseWriter, r *http.Request) {
	l := getLogger(r.Context())
	if !serveFiles {
		handleRedirect(w, r)
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
		handleRedirect(w, r)
	} else {
		http.ServeFile(w, r, path)
		l.Info("served file", slog.Group("response", "status_code", 200, "file", path))
	}
}

func handleRedirect(w http.ResponseWriter, r *http.Request) {
	l := getLogger(r.Context())

	url, err := lookupRedirect(r.URL.Path)
	if err != nil {
		handleNotFound(w, r)
		return
	}

	rg := slog.Group("response", "location", url, "status_code", http.StatusMovedPermanently)
	l.Info("served redirect", rg)

	w.Header().Set("Access-Control-Allow-Origin", "*")
	http.Redirect(w, r, url, http.StatusMovedPermanently)
}

// handleNotFound handles invalid paths/redirects and returns a 404.html or plaintext "Not found"
func handleNotFound(w http.ResponseWriter, r *http.Request) {
	l := getLogger(r.Context())
	logPlainResponse := func() {
		l.Error("not found", slog.Group("response", "status_code", 404, "text", "Not found"))
	}

	w.WriteHeader(http.StatusNotFound)

	if !serveFiles {
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

// lookupRedirect checks if an alias/redirect has been specified and returns it
// if not found, this method will update the list of redirects
func lookupRedirect(path string) (string, error) {
	// Remove the leading / from the path
	alias := strings.TrimPrefix(path, "/")

	// Lookup the redirect and return the URL if found
	if url, exists := redirects[alias]; exists {
		return url, nil
	}

	// Redirect not found, so let's update the list
	newRedirects, err := fetchRedirects()
	if err != nil {
		// Return error but don't exit the program - this will leave the
		// existing map in place which should still work fine.
		logger.Error("failed to update redirect map: %s", err.Error())
		return "", fmt.Errorf("redirect not found")
	}
	redirects = newRedirects

	// Check again, if redirect now exists then return the URL
	if url, exists := redirects[alias]; exists {
		return url, nil
	}

	return "", fmt.Errorf("redirect not found")
}

// fetchRedirects gets the latest redirects file from the specified url
func fetchRedirects() (map[string]string, error) {
	// Add a query param to the URL to break caching if required (Github Gists!)
	reqURL := fmt.Sprintf("%s?cachebust=%d", os.Getenv("GOSHERVE_REDIRECT_MAP_URL"), time.Now().Unix())

	resp, err := http.Get(reqURL)
	logger.Debug("fetched redirects specification", "url", reqURL)
	if err != nil {
		return nil, fmt.Errorf("error fetching redirects from %s", reqURL)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading redirect gist")
	}

	gistRedirects := make(map[string]string)

	for i, line := range strings.Split(string(body), "\n") {
		// Ignore blank lines
		if len(line) == 0 {
			continue
		}

		parts := strings.Split(line, " ")
		// Reject the line if there is more than one space
		if len(parts) != 2 {
			logger.Debug("invalid redirect specification", "line", i+1)
			continue
		}

		// Check the second part is actually a valid URL
		if _, err := url.Parse(parts[1]); err != nil {
			logger.Debug("invalid url detected in redirects file", "line", i+1, "url", parts[1])
		} else {
			// Naive parsing complete, add redirect to the map
			gistRedirects[parts[0]] = parts[1]
			rg := slog.Group("redirect", "alias", parts[0], "url", parts[1])
			logger.Debug("updated redirect", rg)
		}
	}
	return gistRedirects, nil
}

// requestLogger is a middleware that injects a logger into the request's
// context which automatically includes a log group with request information
func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		l := logger.With(slog.Group("request", "method", r.Method, "url", r.URL.Path))
		ctx := context.WithValue(r.Context(), "logger", l)
		next.ServeHTTP(rw, r.WithContext(ctx))
	})
}

// getLogger is a helper for pulling a logger from a context value
func getLogger(ctx context.Context) *slog.Logger {
	l, ok := ctx.Value("logger").(*slog.Logger)
	if !ok {
		// If we can't get a logger from the context, return the global logger
		return logger
	}
	return l
}

// getRootLogger builds a new slog.Logger which is configured at the log level
// according to the GOSHERVE_LOG_LEVEL environment variable
func getRootLogger(inputLevel string) *slog.Logger {
	logLevel := new(slog.LevelVar)
	h := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})
	logger = slog.New(h)
	slog.SetDefault(logger)

	levelNames := map[string]slog.Level{
		"":      slog.LevelInfo,
		"debug": slog.LevelDebug,
		"info":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
	}

	level, _ := levelNames[inputLevel]
	logLevel.Set(level)
	return logger
}
