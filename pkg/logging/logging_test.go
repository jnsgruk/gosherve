package logging

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { check.TestingT(t) }

type LoggingTestSuite struct {
	logger *slog.Logger
}

func (s *LoggingTestSuite) SetUpSuite(c *check.C) {
	s.logger = SetupLogger("info")
}

var _ = check.Suite(&LoggingTestSuite{})

// TestGetRootLoggerDefaults ensures that when there is no log level
// set by viper (which is bound to an env var in real life), that the
// logger defaults to the INFO level.
func (s *LoggingTestSuite) TestGetRootLoggerDefaults(c *check.C) {
	ctx := context.Background()
	c.Assert(s.logger.Enabled(ctx, slog.LevelInfo), check.Equals, true)
	c.Assert(s.logger.Enabled(ctx, slog.LevelDebug), check.Equals, false)
}

// TestGetRootLoggerSetLogLevel ensures that when the "log_level"
// key is manipulated in viper, the level of the root logger is
// manipulated accordingly.
func (s *LoggingTestSuite) TestGetRootLoggerSetLogLevel(c *check.C) {
	ctx := context.Background()
	s.logger = SetupLogger("DEBUG")
	c.Assert(s.logger.Enabled(ctx, slog.LevelDebug), check.Equals, true)

	s.logger = SetupLogger("info")
	c.Assert(s.logger.Enabled(ctx, slog.LevelDebug), check.Equals, false)
}

// TestGetLoggerFromCtxSuccess ensures that the correct logger is
// pulled from a context which carries a slog.Logger associated with
// the "logger" value.
func (s *LoggingTestSuite) TestGetLoggerFromCtxSuccess(c *check.C) {
	ctxLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ctx := context.WithValue(context.Background(), ctxLoggerKey, ctxLogger)
	loggerFromCtx := GetLoggerFromCtx(ctx)
	c.Assert(loggerFromCtx, check.Equals, ctxLogger)
}

// TestGetLoggerFromCtxFailure ensures that when the specified context
// carries no logger, the default root logger is returned.
func (s *LoggingTestSuite) TestGetLoggerFromCtxFailure(c *check.C) {
	ctx := context.Background()
	loggerFromCtx := GetLoggerFromCtx(ctx)
	c.Assert(loggerFromCtx, check.Equals, s.logger)
}

// TestRequestLoggerMiddleware ensures that a logger is attached
// to the context of each request, and that logger includes a group
// named "request" which contains "method" and "url" fields.
func (s *LoggingTestSuite) TestRequestLoggerMiddleware(c *check.C) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		val := r.Context().Value(ctxLoggerKey)
		if val == nil {
			c.Error("logger not present")
			c.FailNow()
		}
		ctxLogger, ok := val.(*slog.Logger)
		if !ok {
			c.Error("not a *slog.Logger")
			c.FailNow()
		}
		ctxLogger.Info("test")
	})

	// Setup a logger that writes to a buffer
	var buf bytes.Buffer
	h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(h)
	slog.SetDefault(logger)

	// Create a handler to test that uses our "nextHandler"
	handlerToTest := RequestLoggerMiddleware(nextHandler)
	// Make a recorded HTTP request
	req := httptest.NewRequest("GET", "http://testing/foo", nil)
	handlerToTest.ServeHTTP(httptest.NewRecorder(), req)

	// Check that log lines written contain the "request" group
	ok := strings.Contains(buf.String(), `"request":{"method":"GET","url":"/foo","user_agent":""}`)
	c.Assert(ok, check.Equals, true)
}
