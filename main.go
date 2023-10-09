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
var redirects map[string]string
var serveFiles bool = true

func main() {
	// Check for redirect map URL, exit application if absent
	if _, set := os.LookupEnv("REDIRECT_MAP_URL"); !set {
		logger.Error("REDIRECT_MAP_URL environment variable not set")
		os.Exit(1)
	}

	// Check if gosherve is required to serve files from a directory.
	if _, set := os.LookupEnv("WEBROOT"); !set {
		logger.Info("WEBROOT environment variable not set")
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

	http.HandleFunc("/", routeHandler)
	http.ListenAndServe(":8080", nil)
}

// routeHandler is the initial URL handler for all paths
func routeHandler(w http.ResponseWriter, r *http.Request) {
	if !serveFiles {
		handleRedirect(w, r)
		return
	}

	var path string
	if r.URL.Path == "/" {
		path = os.Getenv("WEBROOT") + "/index.html"
	} else {
		path = os.Getenv("WEBROOT") + r.URL.Path
	}

	// Check if the requested file is in the defined webroot
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// If the URL doesn't represent a valid file, treat request as a redirect
		handleRedirect(w, r)
	} else {
		http.ServeFile(w, r, path)
		logger.Info("serving file", "method", r.Method, "path", r.URL.Path, "status_code", 200)
	}
}

func handleRedirect(w http.ResponseWriter, r *http.Request) {
	url, err := lookupRedirect(r.URL.Path)
	if err != nil {
		handleNotFound(w, r)
		return
	}

	logger.Info(
		"serving redirect",
		"method", r.Method,
		"path", r.URL.Path,
		"redirect_to", url,
		"status_code", 200,
	)

	w.Header().Set("Access-Control-Allow-Origin", "*")
	http.Redirect(w, r, url, http.StatusMovedPermanently)
}

// handleNotFound handles invalid paths/redirects and returns a 404.html or plaintext "Not found"
func handleNotFound(w http.ResponseWriter, r *http.Request) {
	logger.Error("not found", "method", r.Method, "path", r.URL.Path, "status_code", 404)
	w.WriteHeader(http.StatusNotFound)

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

	// Lookup the redirect and return the URL if found
	if url, exists := redirects[alias]; exists {
		return url, nil
	}

	// Redirect not found, so let's update the list
	newRedirects, err := fetchRedirects()
	if err != nil {
		// Return error but don't exit the program - this will leave the
		// existing map in place which should still work fine.
		logger.Error("could not fetch redirect updated map")
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
	reqURL := fmt.Sprintf("%s?cachebust=%d", os.Getenv("REDIRECT_MAP_URL"), time.Now().Unix())

	resp, err := http.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("error getting redirects from %s", reqURL)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading redirect gist")
	}

	gistRedirects := make(map[string]string)

	for _, line := range strings.Split(string(body), "\n") {
		parts := strings.Split(line, " ")
		// Reject the line if there is more than one space
		if len(parts) != 2 {
			logger.Error("invalid redirect specification", "specification", line)
			continue
		}

		// Check the second part is actually a valid URL
		if _, err := url.Parse(parts[1]); err != nil {
			logger.Error("invalid url detected in redirects file", "url", parts[1])
		} else {
			// Naive parsing complete, add redirect to the map
			gistRedirects[parts[0]] = parts[1]
		}
	}
	return gistRedirects, nil
}
