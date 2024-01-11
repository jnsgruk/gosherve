package server

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"

	"gopkg.in/check.v1"
)

type RouteHandlerTestSuite struct {
	server             *Server
	mockRedirectSource *httptest.Server
}

func (s *RouteHandlerTestSuite) SetUpTest(c *check.C) {
	s.mockRedirectSource = NewMockRedirectSource()
	s.server = NewServer("", fmt.Sprintf("%s/mockRedirects1", s.mockRedirectSource.URL))
}

func (s *RouteHandlerTestSuite) TearDownTest(c *check.C) {
	s.mockRedirectSource.Close()
}

var _ = check.Suite(&RouteHandlerTestSuite{})

// requestRoute is a simple helper function that makes a mock request to a given
// server on a given path, returning the body and status code.
func requestRoute(s Server, path string) (string, int) {
	// Setup the request and recorder
	req := httptest.NewRequest("GET", path, nil)
	rr := httptest.NewRecorder()
	// Invoke the route handler
	s.routeHandler(rr, req)
	// Grab the result and convert the body to a string
	res := rr.Result()
	body, _ := io.ReadAll(res.Body)
	return string(body), res.StatusCode
}

// TestRouteHandlerSimpleRedirects makes two requests to a well defined redirect when
// the webroot is not enabled. The metrics should increase by two, and 302 should be returned
// in both cases
func (s *RouteHandlerTestSuite) TestRouteHandlerSimpleRedirects(c *check.C) {
	body, code := requestRoute(*s.server, "/foo")

	c.Assert(http.StatusMovedPermanently, check.Equals, code)
	c.Assert(strings.TrimSpace(body), check.Equals, `<a href="http://foo.bar">Moved Permanently</a>.`)
	// Check metrics were incremented properly
	c.Assert(readCounter(s.server.metrics.requestsTotal), check.Equals, float64(1))
	c.Assert(readCounterVec(*s.server.metrics.redirectsServed, "foo"), check.Equals, float64(1))

	body, code = requestRoute(*s.server, "/bar")

	c.Assert(http.StatusMovedPermanently, check.Equals, code)
	c.Assert(strings.TrimSpace(body), check.Equals, `<a href="http://bar.baz">Moved Permanently</a>.`)
	// Check metrics were incremented properly
	c.Assert(readCounter(s.server.metrics.requestsTotal), check.Equals, float64(2))
	c.Assert(readCounterVec(*s.server.metrics.redirectsServed, "bar"), check.Equals, float64(1))
}

// TestRouteHandlerRedirectNotFound tests the request of a non-defined redirect when the
// webroot is disabled.
func (s *RouteHandlerTestSuite) TestRouteHandlerRedirectNotFound(c *check.C) {
	body, code := requestRoute(*s.server, "/undefined")

	c.Assert(http.StatusNotFound, check.Equals, code)
	c.Assert(strings.TrimSpace(body), check.Equals, `Not found`)
	// Check metrics were incremented properly
	c.Assert(readCounter(s.server.metrics.requestsTotal), check.Equals, float64(1))
	c.Assert(readCounterVec(*s.server.metrics.responseStatus, "404"), check.Equals, float64(1))
}

// TestRouteHandlerRedirectNotFoundRich tests the request of a non-defined redirect when the
// webroot is enabled and can serve a 404.html.
func (s *RouteHandlerTestSuite) TestRouteHandlerRedirectNotFoundRich(c *check.C) {
	dir := c.MkDir()
	os.WriteFile(path.Join(dir, "404.html"), []byte("<h1>404</h1>"), 0666)
	s.server.webroot = dir

	body, code := requestRoute(*s.server, "/undefined")

	c.Assert(http.StatusNotFound, check.Equals, code)
	c.Assert(strings.TrimSpace(body), check.Equals, `<h1>404</h1>`)
	// Check metrics were incremented properly
	c.Assert(readCounter(s.server.metrics.requestsTotal), check.Equals, float64(1))
	c.Assert(readCounterVec(*s.server.metrics.responseStatus, "404"), check.Equals, float64(1))
}

// TestFileServeOk tests a request to the root both where there is a webroot enabled,
// and where there is not
func (s *RouteHandlerTestSuite) TestFileServeOk(c *check.C) {
	dir := c.MkDir()
	os.WriteFile(path.Join(dir, "index.html"), []byte("<h1>Gosherve</h1>"), 0666)
	os.WriteFile(path.Join(dir, "script.js"), []byte("alert('script')"), 0666)
	s.server.webroot = dir

	body, code := requestRoute(*s.server, "/")

	c.Assert(http.StatusOK, check.Equals, code)
	c.Assert(strings.TrimSpace(body), check.Equals, `<h1>Gosherve</h1>`)
	// Check metrics were incremented properly
	c.Assert(readCounterVec(*s.server.metrics.responseStatus, "200"), check.Equals, float64(1))

	body, code = requestRoute(*s.server, "/script.js")

	c.Assert(http.StatusOK, check.Equals, code)
	c.Assert(strings.TrimSpace(body), check.Equals, `alert('script')`)
	// Check metrics were incremented properly
	c.Assert(readCounterVec(*s.server.metrics.responseStatus, "200"), check.Equals, float64(2))
}

// TestFileServeNotFound tests a request to a file path where the file is not found
func (s *RouteHandlerTestSuite) TestFileServeNotFound(c *check.C) {
	body, code := requestRoute(*s.server, "/")

	c.Assert(http.StatusNotFound, check.Equals, code)
	c.Assert(strings.TrimSpace(body), check.Equals, `Not found`)
	// Check metrics were incremented properly
	c.Assert(readCounterVec(*s.server.metrics.responseStatus, "404"), check.Equals, float64(1))
}
