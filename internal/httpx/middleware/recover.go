package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/qzq-kiim/shop/internal/httpx/reqctx"
)

// Recover turns a panic into a generic 500. Details stay in the log, keyed by
// request id; the client learns nothing about internals.
func Recover(log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				log.Error("panic recovered",
					slog.String("request_id", reqctx.RequestID(r.Context())),
					slog.Any("panic", rec),
					slog.String("stack", string(debug.Stack())),
				)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}()
			next.ServeHTTP(w, r)
		})
	}
}
