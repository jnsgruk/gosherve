package manager

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// RefreshRedirects is used to refresh the list of configured redirects
// in the manager by fetching the latest copy from the specified source.
func (m *GosherveManager) RefreshRedirects() error {
	redirects, err := m.fetchRedirects()
	if err != nil {
		slog.Error("failed to update redirect map", "error", err.Error())
		return fmt.Errorf("error refreshing redirects")
	}
	m.Redirects = redirects
	m.Metrics.RedirectsTotal.Set(float64(len(m.Redirects)))
	return nil
}

// fetchRedirects gets the latest redirects from the specified url
func (m *GosherveManager) fetchRedirects() (map[string]string, error) {
	// Add a query param to the URL to break caching if required (Github Gists!)
	reqURL := fmt.Sprintf("%s?cachebust=%d", m.redirectsSource, time.Now().Unix())

	resp, err := http.Get(reqURL)
	slog.Debug("fetched redirects specification", "url", reqURL)
	if err != nil {
		return nil, fmt.Errorf("error fetching redirects from %s", reqURL)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading redirect gist")
	}

	redirects := make(map[string]string)

	for i, line := range strings.Split(string(body), "\n") {
		// Ignore blank lines
		if len(line) == 0 {
			continue
		}

		parts := strings.Split(line, " ")
		// Reject the line if there is more than one space
		if len(parts) != 2 {
			slog.Debug("invalid redirect specification", "line", i+1)
			continue
		}

		// Check the second part is actually a valid URL
		if _, err := url.Parse(parts[1]); err != nil {
			slog.Debug("invalid url detected in redirects file", "line", i+1, "url", parts[1])
		} else {
			// Naive parsing complete, add redirect to the map
			redirects[parts[0]] = parts[1]
			rg := slog.Group("redirect", "alias", parts[0], "url", parts[1])
			slog.Debug("updated redirect", rg)
		}
	}
	return redirects, nil
}

// LookupRedirect checks if an alias/redirect has been specified and returns it.
// If not found, this method will update the list of redirects and retry the lookup.
func (m *GosherveManager) LookupRedirect(alias string) (string, error) {
	// Lookup the redirect and return the URL if found
	if url, exists := m.Redirects[alias]; exists {
		return url, nil
	}

	// Redirect not found, so let's update the list
	err := m.RefreshRedirects()
	if err != nil {
		// Return error but don't exit the program - this will leave the
		// existing map in place which should still work fine.
		return "", fmt.Errorf("redirect not found")
	}

	// Check again, if redirect now exists then return the URL
	if url, exists := m.Redirects[alias]; exists {
		return url, nil
	}

	return "", fmt.Errorf("redirect not found")
}
