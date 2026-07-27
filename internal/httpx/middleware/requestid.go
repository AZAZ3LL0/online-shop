package middleware

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/qzq-kiim/shop/internal/httpx/reqctx"
)

// HeaderRequestID is echoed back so a log line can be found from a response.
const HeaderRequestID = "X-Request-Id"

// RequestID puts a fresh id on every request. Inbound ids are ignored: a client
// must not be able to poison the logs by choosing its own.
func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := uuid.NewString()
			w.Header().Set(HeaderRequestID, id)
			next.ServeHTTP(w, r.WithContext(reqctx.WithRequestID(r.Context(), id)))
		})
	}
}
