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

// sign encodes the payload before signing it: a cookie value may not contain
// quotes, commas or spaces, and the first-touch cookie carries JSON.
func (s *Signer) sign(value string) string {
	encoded := base64.RawURLEncoding.EncodeToString([]byte(value))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(s.mac(value))
}

func (s *Signer) verify(signed string) (string, bool) {
	encoded, sig, ok := strings.Cut(signed, ".")
	if !ok {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", false
	}
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		return "", false
	}
	if !hmac.Equal(got, s.mac(string(raw))) {
		return "", false
	}
	return string(raw), true
}

func (s *Signer) mac(value string) []byte {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(value))
	return m.Sum(nil)
}
