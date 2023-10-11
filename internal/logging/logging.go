package logging

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/viper"
)

var logLevels map[string]slog.Level = map[string]slog.Level{
	"":      slog.LevelInfo,
	"debug": slog.LevelDebug,
	"info":  slog.LevelInfo,
	"warn":  slog.LevelWarn,
	"error": slog.LevelError,
}

// GetRootLogger builds a new slog.Logger which is configured at the log level
// according to the GOSHERVE_LOG_LEVEL environment variable
func GetRootLogger() *slog.Logger {
	logLevel := new(slog.LevelVar)
	h := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})
	logger := slog.New(h)
	slog.SetDefault(logger)

	envLevel := viper.GetString("log_level")
	logLevel.Set(logLevels[strings.ToLower(envLevel)])

	return logger
}

// GetLoggerFromCtx is a helper for pulling a logger from a context value
func GetLoggerFromCtx(ctx context.Context) *slog.Logger {
	l, ok := ctx.Value("logger").(*slog.Logger)
	if !ok {
		// If we can't get a logger from the context, return the global logger
		return GetRootLogger()
	}
	return l
}

// requestLogger is a middleware that injects a logger into the request's
// context which automatically includes a log group with request information
func RequestLoggerMiddleware(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		l := logger.With(slog.Group("request", "method", r.Method, "url", r.URL.Path))
		ctx := context.WithValue(r.Context(), "logger", l)
		next.ServeHTTP(rw, r.WithContext(ctx))
	})
}
