package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"log/slog"
)

var logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))

// map to hold defined redirects/aliases
var redirects map[string]string
var serveFiles bool = true

func main() {
	// Set up the web root directory from an env variable or quit
	if _, set := os.LookupEnv("WEBROOT"); !set {
		logger.Info("WEBROOT environment variable not set")
		serveFiles = false
	}
	// Get the Gist that contains the list of redirects
	if _, set := os.LookupEnv("REDIRECT_MAP_URL"); !set {
		logger.Error("REDIRECT_MAP_URL environment variable not set")
		os.Exit(1)
	}
	// Initially populate the map of redirects
	if err := fetchRedirects(); err != nil {
		logger.Error("error fetching redirect map")
	}
	// Add a handler for the root URL
	http.HandleFunc("/", routeHandler)
	// Start listening
	http.ListenAndServe(":8080", nil)
}

// routeHandler is the initial URL handler for all paths
func routeHandler(w http.ResponseWriter, r *http.Request) {
	// Check if file serving is enabled, if not handle redirect and return
	if !serveFiles {
		handleRedirect(w, r)
		return
	}
	// Parse the request path
	var path string
	if r.URL.Path == "/" {
		// Redirect requests to the root to index.html
		path = os.Getenv("WEBROOT") + "/index.html"
	} else {
		// Otherwise just serve the file requested
		path = os.Getenv("WEBROOT") + r.URL.Path
	}
	// Check if the requested file is in the defined webroot
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// If the URL doesn't represent a valid file, see if a redirect is specified
		handleRedirect(w, r)
	} else {
		// Found a file matching the request, return it (no need to sanitise, ServeFile is *clever*)
		http.ServeFile(w, r, path)
		logger.Info(
			"serving file",
			"method", r.Method,
			"path", r.URL.Path,
			"status_code", 200,
		)
	}
}

func handleRedirect(w http.ResponseWriter, r *http.Request) {
	// Check for specified redirects
	url, err := lookupRedirect(r.URL.Path)
	if err != nil {
		// No file or redirect, return a 404
		handleNotFound(w, r)
		return
	}
	// Set the redirect type
	redirectType := http.StatusMovedPermanently

	logger.Info(
		"serving redirect",
		"method", r.Method,
		"path", r.URL.Path,
		"redirect_to", url,
		"status_code", 200,
	)

	w.Header().Set("Access-Control-Allow-Origin", "*")
	http.Redirect(w, r, url, redirectType)
}

// handleNotFound handles invalid paths/redirects and returns a 404.html or plaintext "Not found"
func handleNotFound(w http.ResponseWriter, r *http.Request) {
	logger.Error("not found", "method", r.Method, "path", r.URL.Path, "status_code", 404)
	w.WriteHeader(http.StatusNotFound)

	// If serving files is disabled, return a simple text response
	if !serveFiles {
		w.Write([]byte("Not found"))
		return
	}

	// Check if there is a 404.html to return, otherwise return plaintext
	content, err := os.ReadFile(fmt.Sprintf("%s/%s", os.Getenv("WEBROOT"), "404.html"))
	if err != nil {
		w.Write([]byte("Not found"))
		return
	}
	w.Write(content)
}

// lookupRedirect checks if an alias/redirect has been specified and returns it
// if not found, this method will update the list of redirects
func lookupRedirect(path string) (string, error) {
	// Remove the leading / from the path
	alias := strings.TrimPrefix(path, "/")
	// If the redirect already exists in the map, return its URL
	if url, exists := redirects[alias]; exists {
		return url, nil
	}
	// Redirect not found, so let's update the list
	if err := fetchRedirects(); err != nil {
		logger.Error("error fetching redirect map")
	}
	// Check again, if redirect now exists then return the URL
	if url, exists := redirects[alias]; exists {
		return url, nil
	}
	// Redirect not defined, return an error
	return "", fmt.Errorf("redirect not found")
}

// fetchRedirects gets the latest redirects file from the specified url
func fetchRedirects() error {
	// Add a query param to the URL to break caching if required (Github Gists!)
	reqURL := fmt.Sprintf("%s?cachebust=%d", os.Getenv("REDIRECT_MAP_URL"), time.Now().Unix())
	// Get the redirect list
	resp, err := http.Get(reqURL)
	if err != nil {
		return fmt.Errorf("error getting redirects from %s", reqURL)
	}
	// Read the redirect list
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading redirect gist")
	}
	// Create a new map to hold the redirects just fetched
	gistRedirects := make(map[string]string)
	// Iterate over the file
	for _, line := range strings.Split(string(body), "\n") {
		// Split around spaces
		parts := strings.Split(line, " ")
		// Ensure only two parts to the line
		if len(parts) == 2 {
			// Check the second part is actually a valid URL
			if _, err := url.Parse(parts[1]); err != nil {
				logger.Error("invalid url detected in redirects file", "url", parts[1])
			} else {
				// Naive parsing complete, add redirect to the map
				gistRedirects[parts[0]] = parts[1]
			}
		}
	}
	// Update the global redirects map
	redirects = gistRedirects
	logger.Info("redirect map updated")
	return nil
}
