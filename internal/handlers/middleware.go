package handlers

import (
	"log/slog"
	"net/http"
	"time"
)

func LoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t0 := time.Now()
			rw := &responseWriter{ResponseWriter: w}

			defer func() { // still runs even if panic occurs
				logger.Info("handler",
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.Any("query", r.URL.Query()),
					slog.Int("status", rw.status),
					slog.String("remote", r.RemoteAddr),
					slog.Duration("duration", time.Since(t0)))
			}()

			next.ServeHTTP(w, r)
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture the status code and the
// size of the data written
type responseWriter struct {
	http.ResponseWriter
	status  int
	written bool
}

func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.status = statusCode
	rw.written = true
	rw.ResponseWriter.WriteHeader(statusCode)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.written {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.ResponseWriter.Write(b)
}
