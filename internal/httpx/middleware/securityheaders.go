package middleware

import (
	"net/http"
	"strings"
)

// csp allows only same-origin scripts: Alpine and Chart.js are vendored as
// files, never loaded from a CDN. 'unsafe-eval' is required because Alpine
// evaluates the x-* expressions written in the markup; no inline <script> tag
// is ever allowed.
const csp = "default-src 'self'; " +
	"script-src 'self' 'unsafe-eval'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data:; " +
	"connect-src 'self'; " +
	"font-src 'self'; " +
	"base-uri 'none'; " +
	"form-action 'self'; " +
	"object-src 'none'"

// hstsMaxAge is one year, applied only in production behind TLS.
const hstsMaxAge = "max-age=31536000; includeSubDomains"

// SecurityHeaders sets the response headers required by tech.md §9.7.
// The Mini App is framed by Telegram, so /tgapp gets frame-ancestors instead of
// the blanket X-Frame-Options: DENY the rest of the site uses.
func SecurityHeaders(prod bool) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			policy := csp
			if strings.HasPrefix(r.URL.Path, "/tgapp") {
				policy += "; frame-ancestors https://web.telegram.org"
			} else {
				policy += "; frame-ancestors 'none'"
				h.Set("X-Frame-Options", "DENY")
			}
			h.Set("Content-Security-Policy", policy)
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			if prod {
				h.Set("Strict-Transport-Security", hstsMaxAge)
			}
			next.ServeHTTP(w, r)
		})
	}
}
