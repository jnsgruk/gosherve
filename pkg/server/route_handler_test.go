package server

import (
	"embed"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"

	"gopkg.in/check.v1"
)

//go:embed testdata/embed_a/*.txt testdata/embed_b/*.txt
var embeddedTestFiles embed.FS

type RouteHandlerTestSuite struct {
	server             *Server
	mockRedirectSource *httptest.Server
}

func (s *RouteHandlerTestSuite) SetUpTest(c *check.C) {
	s.mockRedirectSource = NewMockRedirectSource()
	s.server = NewServer(nil, fmt.Sprintf("%s/mockRedirects1", s.mockRedirectSource.URL))
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

// TestRouteHandlerSimpleRedirects makes three requests to a well defined redirect when
// the webroot is not enabled. The metrics should increase by three, and 302 should be returned
// in both cases
func (s *RouteHandlerTestSuite) TestRouteHandlerSimpleRedirects(c *check.C) {

	var redirectTests = []struct {
		urlPath  string
		redirect string
	}{
		{"/foo", "http://foo.bar"},
		{"/bar", "http://bar.baz"},
		{"/bar/", "http://bar.baz"},
	}

	for i, t := range redirectTests {
		body, code := requestRoute(*s.server, t.urlPath)

		c.Assert(http.StatusMovedPermanently, check.Equals, code)
		c.Assert(strings.TrimSpace(body), check.Equals, fmt.Sprintf(`<a href="%s">Moved Permanently</a>.`, t.redirect))
		// Check metrics were incremented properly
		c.Assert(readCounter(s.server.metrics.requestsTotal), check.Equals, float64(i+1))
		c.Assert(readCounterVec(*s.server.metrics.redirectsServed, "foo"), check.Equals, float64(1))
	}
}

// TestRouteHandlerRedirectNotFound tests the request of a non-defined redirect when the
// webroot is disabled.
func (s *RouteHandlerTestSuite) TestRouteHandlerRedirectNotFound(c *check.C) {
	body, code := requestRoute(*s.server, "/undefined")

	c.Assert(code, check.Equals, http.StatusNotFound)
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
	fsys := os.DirFS(dir)
	s.server.webroot = &fsys

	body, code := requestRoute(*s.server, "/undefined")

	c.Assert(code, check.Equals, http.StatusNotFound)
	c.Assert(strings.TrimSpace(body), check.Equals, `<h1>404</h1>`)
	// Check metrics were incremented properly
	c.Assert(readCounter(s.server.metrics.requestsTotal), check.Equals, float64(1))
	c.Assert(readCounterVec(*s.server.metrics.responseStatus, "404"), check.Equals, float64(1))
}

// TestRouteHandlerNotFoundCacheRule ensures custom 404 files use the same
// resolved-path cache rules as other file responses.
func (s *RouteHandlerTestSuite) TestRouteHandlerNotFoundCacheRule(c *check.C) {
	dir := c.MkDir()
	os.WriteFile(path.Join(dir, "404.html"), []byte("<h1>404</h1>"), 0666)
	fsys := os.DirFS(dir)
	s.server = NewServer(&fsys, "", WithCacheRules(
		CacheRule{Pattern: "404.html", CacheControl: "no-store"},
	))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/undefined", nil)
	s.server.routeHandler(rr, req)

	c.Assert(rr.Code, check.Equals, http.StatusNotFound)
	c.Assert(rr.Header().Get("Cache-Control"), check.Equals, "no-store")
	c.Assert(rr.Header().Get("ETag"), check.Not(check.Equals), "")
}

// TestFileServeOk tests a request to the root both where there is a webroot enabled,
// and where there is not
func (s *RouteHandlerTestSuite) TestFileServeOk(c *check.C) {
	dir := c.MkDir()
	os.WriteFile(path.Join(dir, "index.html"), []byte("<h1>Gosherve</h1>"), 0666)
	os.WriteFile(path.Join(dir, "script.js"), []byte("alert('script')"), 0666)
	fsys := os.DirFS(dir)
	s.server.webroot = &fsys

	body, code := requestRoute(*s.server, "/")

	c.Assert(code, check.Equals, http.StatusOK)
	c.Assert(strings.TrimSpace(body), check.Equals, `<h1>Gosherve</h1>`)
	// Check metrics were incremented properly
	c.Assert(readCounterVec(*s.server.metrics.responseStatus, "200"), check.Equals, float64(1))

	body, code = requestRoute(*s.server, "/script.js")

	c.Assert(code, check.Equals, http.StatusOK)
	c.Assert(strings.TrimSpace(body), check.Equals, `alert('script')`)
	// Check metrics were incremented properly
	c.Assert(readCounterVec(*s.server.metrics.responseStatus, "200"), check.Equals, float64(2))
}

// TestDirectoryServeOk tests a request to the root both where there is a webroot enabled,
// and where there is not
func (s *RouteHandlerTestSuite) TestDirectoryServeOk(c *check.C) {
	dir := c.MkDir()
	os.MkdirAll(path.Join(dir, "testDir"), 0777)
	os.WriteFile(path.Join(dir, "testDir", "index.html"), []byte("<h1>Gosherve</h1>"), 0666)

	fsys := os.DirFS(dir)
	s.server.webroot = &fsys

	body, code := requestRoute(*s.server, "/testDir")

	c.Assert(code, check.Equals, http.StatusOK)
	c.Assert(strings.TrimSpace(body), check.Equals, `<h1>Gosherve</h1>`)
	// Check metrics were incremented properly
	c.Assert(readCounterVec(*s.server.metrics.responseStatus, "200"), check.Equals, float64(1))
}

// TestFileServeNotFound tests a request to a file path where the file is not found
func (s *RouteHandlerTestSuite) TestFileServeNotFound(c *check.C) {
	body, code := requestRoute(*s.server, "/")

	c.Assert(code, check.Equals, http.StatusNotFound)
	c.Assert(strings.TrimSpace(body), check.Equals, `Not found`)
	// Check metrics were incremented properly
	c.Assert(readCounterVec(*s.server.metrics.responseStatus, "404"), check.Equals, float64(1))
}

// TestFileServeCache tests that the Cache-Control and ETag headers are set on files served
func (s *RouteHandlerTestSuite) TestFileServeCache(c *check.C) {
	dir := c.MkDir()
	os.WriteFile(path.Join(dir, "index.html"), []byte("<h1>Gosherve</h1>"), 0666)
	fsys := os.DirFS(dir)
	s.server.webroot = &fsys

	// Request the index page initially
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	(*s.server).routeHandler(rr, req)

	// Record the Etag set by the server
	etag := rr.Header().Get("Etag")
	c.Assert(etag, check.Not(check.Equals), "")
	c.Assert(rr.Header().Get("Cache-Control"), check.Equals, DefaultCacheControl)

	// Make another request to same resource with the "If-None-Match" header
	rr = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("If-None-Match", etag)
	(*s.server).routeHandler(rr, req)

	// Ensure that 304 is returned, not 200
	c.Assert(rr.Code, check.Equals, http.StatusNotModified)
}

// TestFileServeCacheRules tests default policies, first-match precedence,
// fingerprinted assets and rules evaluated against resolved directory paths.
func (s *RouteHandlerTestSuite) TestFileServeCacheRules(c *check.C) {
	dir := c.MkDir()
	os.MkdirAll(path.Join(dir, "docs"), 0777)
	os.MkdirAll(path.Join(dir, "feed"), 0777)
	os.MkdirAll(path.Join(dir, "css"), 0777)
	os.WriteFile(path.Join(dir, "index.html"), []byte("home"), 0666)
	os.WriteFile(path.Join(dir, "docs", "index.html"), []byte("docs"), 0666)
	os.WriteFile(path.Join(dir, "feed", "index.xml"), []byte("feed"), 0666)
	os.WriteFile(path.Join(dir, "css", "main.min.abcdef.css"), []byte("css"), 0666)
	os.WriteFile(path.Join(dir, "robots.txt"), []byte("robots"), 0666)
	fsys := os.DirFS(dir)

	s.server = NewServer(&fsys, "", WithCacheRules(
		CacheRule{Pattern: "*.xml", CacheControl: "no-cache"},
		CacheRule{Pattern: "feed/index.xml", CacheControl: "public, max-age=60"},
		CacheRule{Pattern: "docs/index.html", CacheControl: "public, max-age=300"},
		CacheRule{Pattern: "index.html", CacheControl: "no-cache"},
		CacheRule{Pattern: "*.min.*.css", CacheControl: "public, max-age=31536000, immutable"},
	))

	tests := []struct {
		requestPath  string
		cacheControl string
	}{
		{requestPath: "/", cacheControl: "no-cache"},
		{requestPath: "/docs", cacheControl: "public, max-age=300"},
		{requestPath: "/docs/", cacheControl: "public, max-age=300"},
		{requestPath: "/feed/index.xml", cacheControl: "no-cache"},
		{requestPath: "/css/main.min.abcdef.css", cacheControl: "public, max-age=31536000, immutable"},
		{requestPath: "/robots.txt", cacheControl: DefaultCacheControl},
	}

	for _, test := range tests {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("GET", test.requestPath, nil)
		s.server.routeHandler(rr, req)

		c.Assert(rr.Code, check.Equals, http.StatusOK, check.Commentf("request path: %s", test.requestPath))
		c.Assert(rr.Header().Get("Cache-Control"), check.Equals, test.cacheControl, check.Commentf("request path: %s", test.requestPath))
	}
}

// TestNoCacheConditionalRequest ensures no-cache responses retain validators
// and can be revalidated with If-None-Match.
func (s *RouteHandlerTestSuite) TestNoCacheConditionalRequest(c *check.C) {
	dir := c.MkDir()
	os.WriteFile(path.Join(dir, "index.xml"), []byte("<feed></feed>"), 0666)
	fsys := os.DirFS(dir)
	s.server = NewServer(&fsys, "", WithCacheRules(
		CacheRule{Pattern: "*.xml", CacheControl: "no-cache"},
	))

	first := httptest.NewRecorder()
	s.server.routeHandler(first, httptest.NewRequest("GET", "/index.xml", nil))
	etag := first.Header().Get("ETag")
	c.Assert(etag, check.Not(check.Equals), "")
	c.Assert(first.Header().Get("Cache-Control"), check.Equals, "no-cache")

	second := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/index.xml", nil)
	req.Header.Set("If-None-Match", etag)
	s.server.routeHandler(second, req)

	c.Assert(second.Code, check.Equals, http.StatusNotModified)
	c.Assert(second.Header().Get("ETag"), check.Equals, etag)
	c.Assert(second.Header().Get("Cache-Control"), check.Equals, "no-cache")
}

// TestEmbeddedFileETags ensures embedded files use their content, rather than
// filename, size and zero modification time, to generate validators.
func (s *RouteHandlerTestSuite) TestEmbeddedFileETags(c *check.C) {
	embedA, err := fs.Sub(embeddedTestFiles, "testdata/embed_a")
	c.Assert(err, check.IsNil)
	embedB, err := fs.Sub(embeddedTestFiles, "testdata/embed_b")
	c.Assert(err, check.IsNil)

	etagA := calculateETag("same.txt", &embedA)
	etagB := calculateETag("same.txt", &embedB)
	etagOtherName := calculateETag("other.txt", &embedA)

	c.Assert(etagA, check.Not(check.Equals), etagB)
	c.Assert(etagA, check.Equals, etagOtherName)
}
