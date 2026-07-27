package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const SessionCookie = "qzq_session"

// Session issues and validates a signed session cookie. The session id is a
// random UUID; the cookie value is "id.HMAC(id)" so a client cannot forge or
// tamper with another visitor's session id. This makes order-ownership checks
// (see handler.Status) meaningful.
//
// secret must be a stable, non-empty key. secure controls the cookie Secure
// flag (enable in production / behind TLS).
func Session(secret string, secure bool) gin.HandlerFunc {
	key := []byte(secret)
	return func(c *gin.Context) {
		var sessionID string
		if raw, err := c.Cookie(SessionCookie); err == nil {
			sessionID = verify(raw, key)
		}
		if sessionID == "" {
			sessionID = uuid.New().String()
			c.SetSameSite(http.SameSiteLaxMode)
			c.SetCookie(SessionCookie, sign(sessionID, key), 60*60*24*365, "/", "", secure, true)
		}
		c.Set("session_id", sessionID)
		c.Next()
	}
}

func sign(value string, key []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(value))
	return value + "." + hex.EncodeToString(mac.Sum(nil))
}

// verify returns the value if the signature is valid, otherwise "".
func verify(raw string, key []byte) string {
	i := strings.LastIndexByte(raw, '.')
	if i <= 0 {
		return ""
	}
	value, sig := raw[:i], raw[i+1:]
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(value))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(sig)) {
		return ""
	}
	return value
}
