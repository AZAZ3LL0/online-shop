package middleware

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/qzq-kiim/shop/internal/httpx/cookies"
	"github.com/qzq-kiim/shop/internal/httpx/reqctx"
)

// CookieCSRF is the double-submit cookie name.
const CookieCSRF = "csrf"

// FieldCSRF is the form field and header name templates echo the token in.
const (
	FieldCSRF  = "csrf_token"
	HeaderCSRF = "X-CSRF-Token"
)

const csrfTTL = 12 * time.Hour

// maxFormBytes caps how much of a form body is read to find the token.
const maxFormBytes = 1 << 20

// CSRF implements double submit cookie plus an Origin check, tech.md §9.3.
// Webhooks are excluded: they carry HMAC signatures instead and never see a
// browser cookie.
func CSRF(signer *cookies.Signer, baseURL string, exempt ...string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := signer.Get(r, CookieCSRF)
			if !ok {
				token = randomToken()
				signer.Set(w, CookieCSRF, token, csrfTTL)
			}
			ctx := reqctx.WithCSRFToken(r.Context(), token)
			r = r.WithContext(ctx)

			if isSafeMethod(r.Method) || hasPrefix(r.URL.Path, exempt) {
				next.ServeHTTP(w, r)
				return
			}
			if !sameOrigin(r, baseURL) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			if !validToken(r, token) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func isSafeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

func hasPrefix(path string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// sameOrigin accepts a request whose Origin or Referer matches the configured
// base URL. A request with neither header is rejected: browsers always send one
// on a cross-site form post.
func sameOrigin(r *http.Request, baseURL string) bool {
	want, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	candidate := r.Header.Get("Origin")
	if candidate == "" {
		candidate = r.Header.Get("Referer")
	}
	if candidate == "" {
		return false
	}
	got, err := url.Parse(candidate)
	if err != nil {
		return false
	}
	return got.Scheme == want.Scheme && got.Host == want.Host
}

func validToken(r *http.Request, want string) bool {
	got := r.Header.Get(HeaderCSRF)
	if got == "" {
		got = tokenFromBody(r)
	}
	if got == "" || want == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// tokenFromBody reads the token out of a form body and puts the body back for
// the handler. http.Request.ParseForm only reads the body for POST, PUT and
// PATCH, so DELETE has to be handled here.
func tokenFromBody(r *http.Request) string {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		return ""
	}
	if r.Body == nil {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxFormBytes))
	if err != nil {
		return ""
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	values, err := url.ParseQuery(string(body))
	if err != nil {
		return ""
	}
	return values.Get(FieldCSRF)
}

func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand never fails on the platforms this runs on; if it does,
		// serving a guessable token would be worse than crashing.
		panic("csrf: crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}
