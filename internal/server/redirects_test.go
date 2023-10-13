package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"gopkg.in/check.v1"
)

var mockRedirects1 = `
foo http://foo.bar
bar http://bar.baz
garbagethatshouldntbeparsed
`

var mockRedirects2 = `
foo http://foo.bar
bar http://bar.baz
baz http://baz.qux
garbagethatshouldntbeparsed
`

func Test(t *testing.T) { check.TestingT(t) }

type RedirectsTestSuite struct{}

// func (s *ServerTestSuite) SetUpSuite(c *check.C) {}
var _ = check.Suite(&RedirectsTestSuite{})

func NewMockRedirectSource() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/mockRedirects1" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(mockRedirects1))
			return
		}
		if r.URL.Path == "/mockRedirects2" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(mockRedirects2))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte{})
	}))
}

// readGauge is a helper function for reading prometheus gauge values
func readGauge(m prometheus.Gauge) float64 {
	pb := &dto.Metric{}
	m.Write(pb)
	return pb.GetGauge().GetValue()
}

// TestFetchRedirectsOnInit tests the happy path of a serve being initialised and hydrating
// itself with an initial set of redirects
func (s *RedirectsTestSuite) TestFetchRedirectsOnInit(c *check.C) {
	mockServer := NewMockRedirectSource()
	defer mockServer.Close()

	server := NewServer("", fmt.Sprintf("%s/mockRedirects1", mockServer.URL))
	c.Assert(server.redirects, check.DeepEquals, map[string]string{})
	c.Assert(readGauge(server.metrics.redirectsDefined), check.Equals, float64(0))

	err := server.RefreshRedirects()

	c.Assert(err, check.IsNil)
	c.Assert(server.redirects, check.DeepEquals, map[string]string{
		"foo": "http://foo.bar",
		"bar": "http://bar.baz",
	})
	c.Assert(readGauge(server.metrics.redirectsDefined), check.Equals, float64(2))
}

// TestRedirectsUpdate tests that refreshing redirects to add more redirects works
func (s *RedirectsTestSuite) TestRedirectsUpdate(c *check.C) {
	mockServer := NewMockRedirectSource()
	defer mockServer.Close()

	server := NewServer("", fmt.Sprintf("%s/mockRedirects1", mockServer.URL))
	err := server.RefreshRedirects()

	c.Assert(err, check.IsNil)
	c.Assert(readGauge(server.metrics.redirectsDefined), check.Equals, float64(2))

	server.redirectsSource = fmt.Sprintf("%s/mockRedirects2", mockServer.URL)
	err = server.RefreshRedirects()

	c.Assert(err, check.IsNil)
	c.Assert(readGauge(server.metrics.redirectsDefined), check.Equals, float64(3))

	c.Assert(server.redirects, check.DeepEquals, map[string]string{
		"foo": "http://foo.bar",
		"bar": "http://bar.baz",
		"baz": "http://baz.qux",
	})
}

// TestRedirectsUpdateFailedHydrate tests the error response when a hydration fails
func (s *RedirectsTestSuite) TestRedirectsUpdateFailedHydrate(c *check.C) {
	server := NewServer("", "badurl")
	err := server.RefreshRedirects()

	c.Assert(err, check.ErrorMatches, "error refreshing redirects")
	c.Assert(readGauge(server.metrics.redirectsDefined), check.Equals, float64(0))
}

// TestRedirectsUpdateFailedRefresh tests that if there are already redirects defined, a failed
// refresh leaves the existing redirects in place
func (s *RedirectsTestSuite) TestRedirectsUpdateFailedRefresh(c *check.C) {
	mockServer := NewMockRedirectSource()
	defer mockServer.Close()

	server := NewServer("", fmt.Sprintf("%s/mockRedirects1", mockServer.URL))
	err := server.RefreshRedirects()

	c.Assert(err, check.IsNil)
	c.Assert(readGauge(server.metrics.redirectsDefined), check.Equals, float64(2))

	server.redirectsSource = "badurl"
	err = server.RefreshRedirects()

	c.Assert(err, check.ErrorMatches, "error refreshing redirects")
	c.Assert(readGauge(server.metrics.redirectsDefined), check.Equals, float64(2))
}

// TestLookupRedirectPresent tests that LookupRedirect does the right thing when the requested
// redirect is present in the map
func (s *RedirectsTestSuite) TestLookupRedirectPresent(c *check.C) {
	server := NewServer("", "test")
	server.redirects = map[string]string{"foo": "http://foo.bar"}
	redirect, err := server.LookupRedirect("foo")

	c.Assert(err, check.IsNil)
	c.Assert(redirect, check.Equals, "http://foo.bar")
}

// TestLookupRedirectPresentAfterRefresh tests that LookupRedirect fails to find a redirect
// initially but is able to return it after refreshing
func (s *RedirectsTestSuite) TestLookupRedirectPresentAfterRefresh(c *check.C) {
	mockServer := NewMockRedirectSource()
	defer mockServer.Close()

	server := NewServer("", fmt.Sprintf("%s/mockRedirects1", mockServer.URL))
	c.Assert(readGauge(server.metrics.redirectsDefined), check.Equals, float64(0))

	redirect, err := server.LookupRedirect("foo")

	c.Assert(err, check.IsNil)
	c.Assert(redirect, check.Equals, "http://foo.bar")
}

// TestLookupRedirectFail tests that LookupRedirect returns the correct error when a redirect
// cannot be looked up
func (s *RedirectsTestSuite) TestLookupRedirectFail(c *check.C) {
	mockServer := NewMockRedirectSource()
	defer mockServer.Close()

	server := NewServer("", fmt.Sprintf("%s/mockRedirects1", mockServer.URL))
	c.Assert(readGauge(server.metrics.redirectsDefined), check.Equals, float64(0))

	redirect, err := server.LookupRedirect("notpresent")

	c.Assert(redirect, check.Equals, "")
	c.Assert(err, check.ErrorMatches, "redirect not found")
}
