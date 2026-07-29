package httpx_test

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/qzq-kiim/shop/internal/auth"
	"github.com/qzq-kiim/shop/internal/storage/postgres"
)

// The administrator the admin tests sign in as. This is not a credential of any
// deployment: the account is created inside the test database and thrown away
// with the container.
const (
	testAdminLogin    = "root"
	testAdminPassword = "correct-horse-battery-staple"
)

// adminPaths is every page the session guard has to cover. Each admin slice
// adds its own route here, so the guard is never checked on one page only.
var adminPaths = []string{"/admin"}

// createAdmin puts one administrator into the test database.
func createAdmin(t *testing.T, env *shopEnv) {
	t.Helper()
	hash, err := auth.Hash(testAdminPassword)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if _, err := postgres.NewAdminRepo(env.store).Upsert(context.Background(), testAdminLogin, hash); err != nil {
		t.Fatalf("create admin: %v", err)
	}
}

// newClient is a browser with its own cookie jar, so one test can hold several
// independent sessions.
func newClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	return &http.Client{Jar: jar, Timeout: 20 * time.Second}
}

// signIn creates the administrator and returns a signed-in browser.
func signIn(t *testing.T, env *shopEnv) *http.Client {
	t.Helper()
	createAdmin(t, env)

	client := newClient(t)
	status, body := attemptLogin(t, env, client, testAdminPassword)
	if status != http.StatusOK {
		t.Fatalf("admin login = %d, want 200 after the redirect; body: %s", status, body)
	}
	return client
}

// attemptLogin submits the login form once and returns the final response.
func attemptLogin(t *testing.T, env *shopEnv, client *http.Client, password string) (int, string) {
	t.Helper()
	status, form := get(t, client, env.server.URL+"/admin/login")
	if status != http.StatusOK {
		t.Fatalf("GET /admin/login = %d", status)
	}
	return send(t, client, http.MethodPost, env.server.URL+"/admin/login", env.server.URL, url.Values{
		"csrf_token": {capture(t, reCSRF, form, "csrf token")},
		"login":      {testAdminLogin},
		"password":   {password},
	})
}

func TestAdminLoginOpensASessionForTheWholePanel(t *testing.T) {
	env := startShopEnv(t)
	client := signIn(t, env)

	// The session, not the login form, is what opens the panel: every admin
	// page has to answer this browser now.
	for _, path := range adminPaths {
		status, body := get(t, client, env.server.URL+path)
		if status != http.StatusOK {
			t.Fatalf("GET %s with a session = %d", path, status)
		}
		if !strings.Contains(body, testAdminLogin) {
			t.Errorf("GET %s does not show who is signed in", path)
		}
	}
}

func TestAdminPanelIsClosedWithoutASession(t *testing.T) {
	env := startShopEnv(t)
	client := newClient(t)
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	for _, path := range adminPaths {
		status, _ := get(t, client, env.server.URL+path)
		if status != http.StatusSeeOther {
			t.Errorf("GET %s without a session = %d, want 303 to the login form", path, status)
		}
	}
}

func TestAdminLoginRejectsAWrongPassword(t *testing.T) {
	env := startShopEnv(t)
	createAdmin(t, env)

	status, body := attemptLogin(t, env, newClient(t), "not-the-password")
	if status != http.StatusUnauthorized {
		t.Fatalf("wrong password = %d, want 401", status)
	}
	// The answer must not tell an attacker whether the login exists.
	if !strings.Contains(body, "Wrong login or password.") {
		t.Errorf("the rejection is not the neutral one: %s", body)
	}
}

// TestAdminLoginRateLimitStopsBruteForce is the error path of tech.md §9.6:
// five attempts per fifteen minutes per IP, and the sixth is refused even when
// it finally carries the right password.
func TestAdminLoginRateLimitStopsBruteForce(t *testing.T) {
	env := startShopEnv(t)
	createAdmin(t, env)
	client := newClient(t)

	const budget = 5
	for attempt := 1; attempt <= budget; attempt++ {
		status, _ := attemptLogin(t, env, client, "not-the-password")
		if status != http.StatusUnauthorized {
			t.Fatalf("attempt %d = %d, want 401 inside the budget", attempt, status)
		}
	}

	status, _ := attemptLogin(t, env, client, testAdminPassword)
	if status != http.StatusTooManyRequests {
		t.Fatalf("attempt %d = %d, want 429 once the budget is spent", budget+1, status)
	}

	// The refusal must be the rate limit, not a session: the blocked attempt
	// left the browser without one.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if status, _ := get(t, client, env.server.URL+"/admin"); status != http.StatusSeeOther {
		t.Fatalf("GET /admin after a blocked login = %d, want 303", status)
	}
}
