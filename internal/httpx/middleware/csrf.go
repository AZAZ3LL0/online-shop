package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
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
		if err := r.ParseForm(); err != nil {
			return false
		}
		got = r.PostForm.Get(FieldCSRF)
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
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
