package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// stubHealth is the liveness dependency with a switchable database.
type stubHealth struct{ err error }

func (s stubHealth) Ping(context.Context) error { return s.err }

// The probe is what the monitor and Caddy poll, so a database the pool can no
// longer reach has to read as 503 and not as a cheerful 200 (TASKS.md S8.2).
func TestHealthz(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		ping  error
		want  int
		bodyC string
	}{
		{name: "database answers", ping: nil, want: http.StatusOK, bodyC: "ok"},
		{name: "pool is severed", ping: errors.New("conn closed"), want: http.StatusServiceUnavailable, bodyC: "unavailable"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			healthz(stubHealth{err: tc.ping})(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

			if rec.Code != tc.want {
				t.Fatalf("GET /healthz = %d, want %d", rec.Code, tc.want)
			}
			if body := rec.Body.String(); !strings.Contains(body, tc.bodyC) {
				t.Fatalf("GET /healthz body = %q, want it to contain %q", body, tc.bodyC)
			}
		})
	}
}

// The probe answers before it knows anything about the caller, so it must not
// leak the reason the database is unreachable (tech.md §9.13).
func TestHealthzHidesTheDatabaseError(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	healthz(stubHealth{err: errors.New("dial tcp 10.0.0.7:5432: connect: refused")})(
		rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if strings.Contains(rec.Body.String(), "10.0.0.7") {
		t.Fatalf("probe leaks the database address: %q", rec.Body.String())
	}
}
