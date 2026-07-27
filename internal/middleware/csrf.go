package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	CSRFCookie      = "csrf_token"
	CSRFContextKey  = "csrf_token"
	csrfHeader      = "X-CSRF-Token"
	csrfFormField   = "csrf_token"
	csrfTokenLength = 32
)

// CSRF implements stateless CSRF protection. A random token is stored in a
// cookie and, for every rendered page, echoed into forms as a hidden field
// (see render()). On unsafe requests the submitted token (form field or
// X-CSRF-Token header) must match the cookie. A cross-site attacker can neither
// read the victim's cookie nor set the matching field, so forged POSTs fail.
//
// exemptPaths are skipped (e.g. the NowPayments webhook, which authenticates
// via HMAC signature instead).
func CSRF(secure bool, exemptPaths ...string) gin.HandlerFunc {
	exempt := make(map[string]struct{}, len(exemptPaths))
	for _, p := range exemptPaths {
		exempt[p] = struct{}{}
	}

	return func(c *gin.Context) {
		token, err := c.Cookie(CSRFCookie)
		if err != nil || !validToken(token) {
			token = newToken()
			c.SetSameSite(http.SameSiteLaxMode)
			c.SetCookie(CSRFCookie, token, 60*60*24*365, "/", "", secure, true)
		}
		c.Set(CSRFContextKey, token)

		if isUnsafe(c.Request.Method) {
			if _, ok := exempt[c.FullPath()]; !ok {
				submitted := c.GetHeader(csrfHeader)
				if submitted == "" {
					submitted = c.PostForm(csrfFormField)
				}
				if subtle.ConstantTimeCompare([]byte(submitted), []byte(token)) != 1 {
					c.AbortWithStatus(http.StatusForbidden)
					return
				}
			}
		}

		c.Next()
	}
}

func isUnsafe(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

func newToken() string {
	b := make([]byte, csrfTokenLength)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func validToken(t string) bool {
	return len(t) == csrfTokenLength*2
}
