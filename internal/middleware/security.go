package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// contentSecurityPolicy restricts resource loading. htmx is served locally from
// /static; fonts come from Google Fonts. No inline/eval scripts are permitted —
// keep interactivity in the bundled /static/js files.
//
// Note: a few templates use small inline <script> blocks and inline event
// handlers (onclick=...). 'unsafe-inline' for script/style is required until
// those are refactored out; it is a deliberate, documented trade-off.
const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self' 'unsafe-inline'; " +
	"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; " +
	"font-src 'self' https://fonts.gstatic.com; " +
	"img-src 'self' data:; " +
	"connect-src 'self'; " +
	"frame-ancestors 'none'; " +
	"base-uri 'self'; " +
	"form-action 'self' https://nowpayments.io https://*.nowpayments.io"

// SecurityHeaders sets common hardening response headers. HSTS is only emitted
// in production, where the app is served over TLS behind Nginx.
func SecurityHeaders(production bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		if production {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		c.Next()
	}
}

// BodyLimit rejects request bodies larger than max bytes (protects against
// memory-exhaustion via oversized form / cart_json posts).
func BodyLimit(max int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, max)
		c.Next()
	}
}
