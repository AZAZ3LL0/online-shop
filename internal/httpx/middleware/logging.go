package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/qzq-kiim/shop/internal/httpx/reqctx"
)

// Logging writes one structured line per request. Query strings and bodies are
// left out: they carry customer data and secrets.
func Logging(log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w}
			next.ServeHTTP(sw, r)
			if sw.status == 0 {
				sw.status = http.StatusOK
			}
			log.Info("request",
				slog.String("request_id", reqctx.RequestID(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", sw.status),
				slog.Int("bytes", sw.bytes),
				slog.Duration("took", time.Since(start)),
			)
		})
	}
}
