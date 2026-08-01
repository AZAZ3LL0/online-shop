package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/qzq-kiim/shop/internal/httpx/middleware"
)

// The acceptance criterion of tech.md §9.6 is that a limit counts one client,
// so the address has to survive the proxy hop and must not be forgeable by the
// client itself.
func TestClientIPReadsTheProxyChainOnlyFromTheProxy(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		remoteAddr string
		forwarded  string
		want       string
	}{
		{
			name:       "a request straight from the internet is its own peer",
			remoteAddr: "203.0.113.7:41234",
			want:       "203.0.113.7",
		},
		{
			name:       "a forged header from the internet is ignored",
			remoteAddr: "203.0.113.7:41234",
			forwarded:  "198.51.100.1",
			want:       "203.0.113.7",
		},
		{
			name:       "behind the proxy the header carries the client",
			remoteAddr: "172.18.0.4:8080",
			forwarded:  "198.51.100.1",
			want:       "198.51.100.1",
		},
		{
			name:       "the rightmost address the proxy did not add wins",
			remoteAddr: "172.18.0.4:8080",
			forwarded:  "198.51.100.1, 203.0.113.9",
			want:       "203.0.113.9",
		},
		{
			name:       "a prefix the client forged itself never wins",
			remoteAddr: "172.18.0.4:8080",
			forwarded:  "10.0.0.1, 203.0.113.9",
			want:       "203.0.113.9",
		},
		{
			name:       "a chain of private hops falls back to the peer",
			remoteAddr: "172.18.0.4:8080",
			forwarded:  "10.0.0.1, 192.168.1.2",
			want:       "172.18.0.4",
		},
		{
			name:       "a malformed entry stops the walk",
			remoteAddr: "172.18.0.4:8080",
			forwarded:  "203.0.113.9, not-an-address",
			want:       "172.18.0.4",
		},
		{
			name:       "no header behind the proxy is the peer",
			remoteAddr: "172.18.0.4:8080",
			want:       "172.18.0.4",
		},
		{
			name:       "loopback counts as a hop we control",
			remoteAddr: "127.0.0.1:8080",
			forwarded:  "198.51.100.1",
			want:       "198.51.100.1",
		},
		{
			name:       "ipv6 survives the hop",
			remoteAddr: "[fd00::1]:8080",
			forwarded:  "2001:db8::5",
			want:       "2001:db8::5",
		},
		{
			name:       "an ipv4-mapped hop is still a private hop",
			remoteAddr: "172.18.0.4:8080",
			forwarded:  "198.51.100.1, ::ffff:10.0.0.7",
			want:       "198.51.100.1",
		},
		{
			name:       "an address without a port is used as it is",
			remoteAddr: "203.0.113.7",
			want:       "203.0.113.7",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tc.remoteAddr
			if tc.forwarded != "" {
				r.Header.Set(middleware.HeaderForwardedFor, tc.forwarded)
			}
			if got := middleware.ClientIP(r); got != tc.want {
				t.Errorf("ClientIP() = %q, want %q", got, tc.want)
			}
		})
	}
}

// The bug this guards against: with every request arriving from the same proxy,
// one buyer's checkouts used to exhaust the hourly budget of tech.md §9.6 for
// everybody.
func TestTwoClientsBehindOneProxyGetSeparateBudgets(t *testing.T) {
	t.Parallel()

	limiter := middleware.NewLimiter()
	handler := middleware.RateLimit(limiter)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	post := func(client string) int {
		r := httptest.NewRequest(http.MethodPost, "/checkout", nil)
		r.RemoteAddr = "172.18.0.4:8080"
		r.Header.Set(middleware.HeaderForwardedFor, client)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code
	}

	// tech.md §9.6: POST /checkout is 10 per hour per IP.
	for i := range 10 {
		if code := post("198.51.100.1"); code != http.StatusOK {
			t.Fatalf("checkout %d for the first client = %d, want 200", i+1, code)
		}
	}
	if code := post("198.51.100.1"); code != http.StatusTooManyRequests {
		t.Errorf("the eleventh checkout of one client = %d, want 429", code)
	}
	if code := post("203.0.113.9"); code != http.StatusOK {
		t.Errorf("the first checkout of a second client = %d, want 200", code)
	}
}

// Without a trustworthy address the login limit protects nothing: an attacker
// on the internet could spend everybody's budget by rotating a header.
func TestTheLoginLimitCannotBeSpentByAForgedHeader(t *testing.T) {
	t.Parallel()

	limiter := middleware.NewLimiter()
	handler := middleware.RateLimit(limiter)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	attempt := func(forged string) int {
		r := httptest.NewRequest(http.MethodPost, "/admin/login", nil)
		r.RemoteAddr = "203.0.113.7:41234"
		r.Header.Set(middleware.HeaderForwardedFor, forged)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code
	}

	// tech.md §9.6: 5 attempts per 15 minutes per IP. A rotating header must
	// not buy a sixth.
	for i := range 5 {
		if code := attempt("198.51.100." + strconv.Itoa(i+1)); code != http.StatusOK {
			t.Fatalf("attempt %d = %d, want 200", i+1, code)
		}
	}
	if code := attempt("198.51.100.9"); code != http.StatusTooManyRequests {
		t.Errorf("the sixth attempt from one peer = %d, want 429", code)
	}
}
