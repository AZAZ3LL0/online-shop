package cookies_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/qzq-kiim/shop/internal/httpx/cookies"
)

func TestSignedRoundTrip(t *testing.T) {
	s := cookies.NewSigner([]byte("0123456789abcdef0123456789abcdef"), false)
	w := httptest.NewRecorder()
	s.Set(w, "cart_id", "abc-123", time.Hour)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range w.Result().Cookies() {
		r.AddCookie(c)
	}
	got, ok := s.Get(r, "cart_id")
	if !ok || got != "abc-123" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
}

func TestTamperedCookieIsRejected(t *testing.T) {
	s := cookies.NewSigner([]byte("0123456789abcdef0123456789abcdef"), false)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: "cart_id", Value: "evil.AAAA"})
	if _, ok := s.Get(r, "cart_id"); ok {
		t.Fatal("tampered cookie must not verify")
	}
}

func TestCookieFromAnotherKeyIsRejected(t *testing.T) {
	a := cookies.NewSigner([]byte("0123456789abcdef0123456789abcdef"), false)
	b := cookies.NewSigner([]byte("ffffffffffffffffffffffffffffffff"), false)

	w := httptest.NewRecorder()
	a.Set(w, "cart_id", "abc-123", time.Hour)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range w.Result().Cookies() {
		r.AddCookie(c)
	}
	if _, ok := b.Get(r, "cart_id"); ok {
		t.Fatal("cookie signed with another key must not verify")
	}
}
