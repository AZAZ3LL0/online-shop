package httpx_test

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/qzq-kiim/shop/internal/domain/settings"
)

// storedSetting reads one key back out of the key-value table, the only place a
// setting is allowed to live (tech.md §4).
func storedSetting(t *testing.T, env *shopEnv, key string) (string, bool) {
	t.Helper()
	var value string
	err := env.store.Pool().QueryRow(context.Background(),
		`SELECT value::text FROM settings WHERE key = $1`, key).Scan(&value)
	if err != nil {
		return "", false
	}
	return value, true
}

// postSettings submits the settings form.
func postSettings(t *testing.T, env *shopEnv, client *http.Client, shipping, minutes string, paused bool) (int, string) {
	t.Helper()
	_, page := get(t, client, env.server.URL+"/admin/settings")
	form := url.Values{
		"csrf_token":        {capture(t, reCSRF, page, "csrf token")},
		"shipping_cents":    {shipping},
		"order_ttl_minutes": {minutes},
	}
	if paused {
		form.Set("shop_paused", "on")
	}
	return send(t, client, http.MethodPost, env.server.URL+"/admin/settings", env.server.URL, form)
}

// S5.4 acceptance: every known key round-trips through the one key-value
// repository, and what comes back out is what the shop then runs on.
func TestAdminSettingsRoundTripEveryKey(t *testing.T) {
	env := startShopEnv(t)
	client := signIn(t, env)

	// Nothing is stored yet: the page runs on the environment defaults.
	for _, key := range settings.Keys {
		if value, ok := storedSetting(t, env, key); ok {
			t.Fatalf("%s was already stored as %s before anything was saved", key, value)
		}
	}
	status, page := get(t, client, env.server.URL+"/admin/settings")
	if status != http.StatusOK {
		t.Fatalf("GET /admin/settings = %d", status)
	}
	if !strings.Contains(page, `value="30"`) {
		t.Errorf("the form does not show the 30 minute default window: %s", page)
	}

	if status, body := postSettings(t, env, client, "1500", "45", true); status != http.StatusOK {
		t.Fatalf("saving the settings = %d: %s", status, body)
	}
	for key, want := range map[string]string{
		settings.KeyShippingCents:   "1500",
		settings.KeyOrderTTLMinutes: "45",
		settings.KeyShopPaused:      "true",
	} {
		got, ok := storedSetting(t, env, key)
		if !ok {
			t.Errorf("%s was not written", key)
			continue
		}
		if got != want {
			t.Errorf("%s = %s, want %s", key, got, want)
		}
	}

	// Read back through the service the whole shop uses.
	values, err := env.config.Values(context.Background())
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if values.ShippingCents != 1500 || values.TTLMinutes() != 45 || !values.ShopPaused {
		t.Fatalf("settings read back as %+v", values)
	}

	// And the form shows what is stored, not the defaults.
	_, page = get(t, client, env.server.URL+"/admin/settings")
	for _, want := range []string{`value="1500"`, `value="45"`, "checked"} {
		if !strings.Contains(page, want) {
			t.Errorf("the reloaded form does not carry %q", want)
		}
	}

	// Unpausing round-trips as well: a boolean has to be able to go back.
	if status, body := postSettings(t, env, client, "0", "30", false); status != http.StatusOK {
		t.Fatalf("unpausing = %d: %s", status, body)
	}
	if got, _ := storedSetting(t, env, settings.KeyShopPaused); got != "false" {
		t.Errorf("shop_paused = %s, want false", got)
	}
}

// The error path: a value outside the bounds is refused and nothing is stored.
func TestAdminSettingsRefuseValuesOutOfRange(t *testing.T) {
	env := startShopEnv(t)
	client := signIn(t, env)

	for _, c := range []struct{ shipping, minutes string }{
		{"-1", "30"},
		{strconv.Itoa(settings.MaxShippingCents + 1), "30"},
		{"0", "1"},
		{"0", strconv.Itoa(settings.MaxTTLMinutes + 1)},
		{"free", "30"},
		{"0", "half an hour"},
	} {
		status, _ := postSettings(t, env, client, c.shipping, c.minutes, false)
		if status != http.StatusUnprocessableEntity {
			t.Errorf("shipping=%q minutes=%q = %d, want 422", c.shipping, c.minutes, status)
		}
	}
	for _, key := range settings.Keys {
		if value, ok := storedSetting(t, env, key); ok {
			t.Errorf("a refused form still stored %s as %s", key, value)
		}
	}
}

// The settings are not decoration: delivery is priced from them, the reservation
// window is taken from them, and a paused shop stops taking orders.
func TestSettingsDriveTheStorefront(t *testing.T) {
	env := startShopEnv(t)
	client := signIn(t, env)
	if status, body := postSettings(t, env, client, "1500", "45", false); status != http.StatusOK {
		t.Fatalf("saving the settings = %d: %s", status, body)
	}

	// Delivery reaches the cart totals.
	_, home := get(t, env.client, env.server.URL+"/")
	token := capture(t, reCSRF, home, "csrf token")
	variantID := capture(t, reVariant, home, "variant id")
	if status, body := send(t, env.client, http.MethodPost, env.server.URL+"/cart/items", env.server.URL, url.Values{
		"csrf_token": {token},
		"variant_id": {variantID},
		"qty":        {"1"},
	}); status != http.StatusOK {
		t.Fatalf("POST /cart/items = %d: %s", status, body)
	}
	_, cartPage := get(t, env.client, env.server.URL+"/cart")
	if !strings.Contains(cartPage, "$15.00") {
		t.Errorf("the cart does not price delivery from the settings: %s", cartPage)
	}
	// One tee at $35.00 plus $15.00 delivery.
	if !strings.Contains(cartPage, "$50.00") {
		t.Error("the cart total does not include the delivery from the settings")
	}

	// The reservation window reaches the checkout page.
	_, checkoutPage := get(t, env.client, env.server.URL+"/checkout")
	if !strings.Contains(checkoutPage, "45 minutes") {
		t.Errorf("the checkout does not announce the 45 minute window: %s", checkoutPage)
	}

	// Pausing the shop stops the order from being placed at all.
	if status, body := postSettings(t, env, client, "1500", "45", true); status != http.StatusOK {
		t.Fatalf("pausing = %d: %s", status, body)
	}
	status, _, body := sendNoRedirect(t, env, http.MethodPost, "/checkout", checkoutForm(token))
	if status != http.StatusServiceUnavailable {
		t.Fatalf("checkout while paused = %d, want 503", status)
	}
	if !strings.Contains(body, "on pause") {
		t.Errorf("the buyer is not told the shop is paused: %s", body)
	}
	var orders int
	if err := env.store.Pool().QueryRow(context.Background(),
		`SELECT count(*) FROM orders WHERE created_at > now() - interval '1 hour'`).Scan(&orders); err != nil {
		t.Fatalf("count orders: %v", err)
	}
	if orders != 0 {
		t.Fatalf("a paused shop placed %d orders", orders)
	}

	// Unpausing lets the same cart through.
	if status, body := postSettings(t, env, client, "1500", "45", false); status != http.StatusOK {
		t.Fatalf("unpausing = %d: %s", status, body)
	}
	if status, _, body := sendNoRedirect(t, env, http.MethodPost, "/checkout", checkoutForm(token)); status != http.StatusSeeOther {
		t.Fatalf("checkout after unpausing = %d, want 303: %s", status, body)
	}
}
