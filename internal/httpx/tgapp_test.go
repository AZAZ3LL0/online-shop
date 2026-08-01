package httpx_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/qzq-kiim/shop/internal/auth"
	"github.com/qzq-kiim/shop/internal/storage/postgres"
	"github.com/qzq-kiim/shop/internal/telegram"
)

// allowedTelegramID is the account the test environment puts on the allowlist,
// testBotToken is the throwaway token its launches are signed with. Neither
// value exists outside the tests.
const (
	allowedTelegramID = int64(770077)
	testBotToken      = "1234567:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw"
)

// launchAdmin creates an administrator whose Telegram account may open the Mini
// App, and returns the id that was bound to it.
func launchAdmin(t *testing.T, env *shopEnv, telegramID int64) {
	t.Helper()
	const login = "tgapp-admin"
	// The password never leaves the test; the Mini App does not use it at all.
	hash, err := auth.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	admins := postgres.NewAdminRepo(env.store)
	if _, err := admins.Upsert(context.Background(), login, hash); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if err := admins.SetTelegramID(context.Background(), login, &telegramID); err != nil {
		t.Fatalf("bind telegram id: %v", err)
	}
}

// initData signs a launch payload the way the Telegram client would.
func initData(t *testing.T, userID int64, authDate time.Time) string {
	t.Helper()
	user, err := json.Marshal(map[string]any{"id": userID, "username": "az", "first_name": "Samat"})
	if err != nil {
		t.Fatalf("marshal user: %v", err)
	}
	return telegram.SignInitData(url.Values{
		"auth_date": {strconv.FormatInt(authDate.Unix(), 10)},
		"user":      {string(user)},
	}, testBotToken)
}

// launch posts a payload to /tgapp/auth the way the launch page does.
func launch(t *testing.T, client *http.Client, env *shopEnv, form url.Values) (int, map[string]string) {
	t.Helper()
	_, entry := get(t, client, env.server.URL+"/tgapp")
	form.Set("csrf_token", capture(t, reCSRF, entry, "csrf token"))

	status, body := send(t, client, http.MethodPost, env.server.URL+"/tgapp/auth", env.server.URL, form)
	var payload map[string]string
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("decode /tgapp/auth response %q: %v", body, err)
	}
	return status, payload
}

// TestMiniAppLaunchOpensAShortSession is the S7.1 happy path: a fresh, signed
// launch from an allowlisted account gets into the panel.
func TestMiniAppLaunchOpensAShortSession(t *testing.T) {
	env := startShopEnv(t)
	launchAdmin(t, env, allowedTelegramID)
	client := newClient(t)

	status, payload := launch(t, client, env, url.Values{
		"init_data": {initData(t, allowedTelegramID, time.Now().UTC())},
		"theme":     {`{"bg_color":"#17212b","text_color":"#ffffff"}`},
	})
	if status != http.StatusOK {
		t.Fatalf("POST /tgapp/auth = %d, want 200: %v", status, payload)
	}
	if payload["redirect"] != "/tgapp/" {
		t.Fatalf("redirect = %q, want /tgapp/", payload["redirect"])
	}

	// The session cookie is the same one the browser panel uses, so the Mini
	// App reaches the admin pages without a second mechanism.
	status, body := get(t, client, env.server.URL+"/tgapp/")
	if status != http.StatusOK {
		t.Fatalf("GET /tgapp/ after a launch = %d, want 200", status)
	}
	if !strings.Contains(body, "Revenue") {
		t.Error("the mini app dashboard does not show the report")
	}
}

// TestMiniAppServesTheSamePagesInItsOwnLayout is S7.2: one set of handlers,
// two layouts. The Mini App carries no sign-in form and paints itself with the
// colours Telegram sent (tech.md §5.3, §8).
func TestMiniAppServesTheSamePagesInItsOwnLayout(t *testing.T) {
	env := startShopEnv(t)
	launchAdmin(t, env, allowedTelegramID)
	client := newClient(t)

	status, _ := launch(t, client, env, url.Values{
		"init_data": {initData(t, allowedTelegramID, time.Now().UTC())},
		"theme":     {`{"bg_color":"#17212b","text_color":"#ffffff","button_color":"#5288c1"}`},
	})
	if status != http.StatusOK {
		t.Fatalf("launch = %d, want 200", status)
	}

	for _, page := range []struct {
		path    string
		markers []string
	}{
		{"/tgapp/", []string{"Revenue", "Orders", `id="revenue-chart"`, "data-price-marker="}},
		{"/tgapp/analytics", []string{"Funnel", `id="funnel-chart"`, "Traffic sources"}},
	} {
		body := mustGet(t, client, env.server.URL+page.path)
		for _, marker := range page.markers {
			if !strings.Contains(body, marker) {
				t.Errorf("%s does not carry %q, the mini app must show the same report", page.path, marker)
			}
		}
		if !strings.Contains(body, `class="admin-mini`) {
			t.Errorf("%s is not rendered in the AdminMini layout", page.path)
		}
		if strings.Contains(body, `action="/admin/login"`) || strings.Contains(body, `name="password"`) {
			t.Errorf("%s carries a sign-in form, the mini app has none", page.path)
		}
		// The colours come from themeParams, sanitised before they were stored.
		if !strings.Contains(body, "--tg-bg_color:#17212b") {
			t.Errorf("%s is not themed from themeParams", page.path)
		}
		// S7.3: the tables have to stack instead of scrolling sideways, which
		// is what the per-cell labels are for.
		if !strings.Contains(body, `data-label="Source"`) {
			t.Errorf("%s has unlabelled table cells, they cannot stack on a narrow screen", page.path)
		}
		if !strings.Contains(body, `data-metrics-compact="true"`) {
			t.Errorf("%s does not ask for the compact charts", page.path)
		}
	}

	// The browser panel is untouched by all of this.
	browser := signInAdmin(t, env)
	body := mustGet(t, browser, env.server.URL+"/admin")
	if strings.Contains(body, "admin-mini") {
		t.Error("the browser panel picked up the mini app layout")
	}
	if !strings.Contains(body, `data-metrics-compact="false"`) {
		t.Error("the browser panel asks for the compact charts")
	}
}

// TestMiniAppRejectsBadLaunches is the S7.1 error path. Every refusal has to
// answer 403 with the same wording: tech.md §5.5 requires no detail.
func TestMiniAppRejectsBadLaunches(t *testing.T) {
	env := startShopEnv(t)
	launchAdmin(t, env, allowedTelegramID)
	now := time.Now().UTC()

	cases := []struct {
		name string
		data string
	}{
		{"auth_date older than 15 minutes", initData(t, allowedTelegramID, now.Add(-16*time.Minute))},
		{"telegram id outside the allowlist", initData(t, 999001, now)},
		{"signature of another bot", telegram.SignInitData(url.Values{
			"auth_date": {strconv.FormatInt(now.Unix(), 10)},
			"user":      {`{"id":770077}`},
		}, "9999999:another-bot-token")},
		{"empty payload", ""},
	}

	var wordings []string
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := newClient(t)
			status, payload := launch(t, client, env, url.Values{"init_data": {tc.data}})
			if status != http.StatusForbidden {
				t.Fatalf("POST /tgapp/auth = %d, want 403", status)
			}
			wordings = append(wordings, payload["error"])

			// A refused launch opens nothing: the panel bounces the follow-up
			// request straight back to the launch page.
			if body := mustGet(t, client, env.server.URL+"/tgapp/"); !strings.Contains(body, "Checking your Telegram account") {
				t.Error("a refused launch still reached the panel")
			}
		})
	}
	for _, got := range wordings {
		if got != wordings[0] {
			t.Fatalf("refusals are distinguishable: %q vs %q", got, wordings[0])
		}
	}
}

// TestMiniAppAllowlistIsCheckedAgainstTheDatabase covers the account that
// Telegram vouches for but that no administrator owns.
func TestMiniAppAllowlistIsCheckedAgainstTheDatabase(t *testing.T) {
	env := startShopEnv(t)
	// No admin row is created at all: the signature is sound, the account is not.
	client := newClient(t)

	status, _ := launch(t, client, env, url.Values{
		"init_data": {initData(t, allowedTelegramID, time.Now().UTC())},
	})
	if status != http.StatusForbidden {
		t.Fatalf("launch without an admin row = %d, want 403", status)
	}
}

// TestMiniAppPagesAreClosedWithoutASession keeps /tgapp/* behind the same guard
// as the browser panel, tech.md §8.5.
func TestMiniAppPagesAreClosedWithoutASession(t *testing.T) {
	env := startShopEnv(t)
	client := newClient(t)
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	for _, path := range []string{"/tgapp/", "/tgapp/analytics"} {
		status, _ := get(t, client, env.server.URL+path)
		if status != http.StatusSeeOther {
			t.Errorf("GET %s without a session = %d, want 303 back to the launch page", path, status)
		}
	}
}

// mustGet fetches a page and fails the test if it does not answer 200.
func mustGet(t *testing.T, client *http.Client, target string) string {
	t.Helper()
	status, body := get(t, client, target)
	if status != http.StatusOK {
		t.Fatalf("GET %s = %d", target, status)
	}
	return body
}
