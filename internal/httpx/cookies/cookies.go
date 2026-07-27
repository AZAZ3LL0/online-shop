// Package cookies issues and reads HMAC-signed cookies. Every cookie the
// application sets goes through here, so the flags are set in one place.
package cookies

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strings"
	"time"
)

// Signer signs and verifies cookie values with the application secret.
type Signer struct {
	key    []byte
	secure bool
}

// NewSigner builds a signer. secure marks cookies Secure, which is mandatory in
// production and impossible over plain http in local development.
func NewSigner(key []byte, secure bool) *Signer {
	return &Signer{key: key, secure: secure}
}

// Set writes a signed cookie with the project-wide flags.
func (s *Signer) Set(w http.ResponseWriter, name, value string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    s.sign(value),
		Path:     "/",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// Clear removes a cookie.
func (s *Signer) Clear(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// Get returns the cookie value when the signature checks out.
func (s *Signer) Get(r *http.Request, name string) (string, bool) {
	c, err := r.Cookie(name)
	if err != nil {
		return "", false
	}
	return s.verify(c.Value)
}

func (s *Signer) sign(value string) string {
	return value + "." + base64.RawURLEncoding.EncodeToString(s.mac(value))
}

func (s *Signer) verify(signed string) (string, bool) {
	idx := strings.LastIndex(signed, ".")
	if idx < 0 {
		return "", false
	}
	value, sig := signed[:idx], signed[idx+1:]
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		return "", false
	}
	if !hmac.Equal(got, s.mac(value)) {
		return "", false
	}
	return value, true
}

func (s *Signer) mac(value string) []byte {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(value))
	return m.Sum(nil)
}
