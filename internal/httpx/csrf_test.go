package httpx_test

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// otherOrigin is a host the shop is not served on. A form posted from it is
// exactly what the Origin half of tech.md §9.3 exists to refuse.
const otherOrigin = "https://not-the-shop.example"

// A refused form is the one 4xx a visitor meets without having done anything
// wrong, so it has to arrive as a page that says what to do, not as the bare
// word "forbidden" that reads like a broken site.
func TestARefusedStorefrontFormExplainsItself(t *testing.T) {
	env := startShopEnv(t)

	_, home := get(t, env.client, env.server.URL+"/")
	token := capture(t, reCSRF, home, "csrf token")
	variantID := capture(t, reVariant, home, "variant id")

	status, body := send(t, env.client, http.MethodPost, env.server.URL+"/cart/items", otherOrigin, url.Values{
		"csrf_token": {token},
		"variant_id": {variantID},
		"qty":        {"1"},
	})
	if status != http.StatusForbidden {
		t.Fatalf("a form from another origin = %d, want 403", status)
	}
	if strings.TrimSpace(body) == "forbidden" {
		t.Fatal("the refusal is still the bare forbidden text")
	}
	for _, want := range []string{"This page has expired", "Reload the page"} {
		if !strings.Contains(body, want) {
			t.Errorf("the refusal does not say %q: %s", want, body)
		}
	}
	// It is a storefront page, so it carries the shop chrome.
	if !strings.Contains(body, "</html>") {
		t.Error("the refusal is not a rendered page")
	}
}

// The cart swaps the answer into a node, so a whole page would land inside the
// cart box. Those calls get a compact alert instead.
func TestARefusedCartFetchGetsAnAlertNotAPage(t *testing.T) {
	env := startShopEnv(t)

	_, home := get(t, env.client, env.server.URL+"/")
	token := capture(t, reCSRF, home, "csrf token")
	variantID := capture(t, reVariant, home, "variant id")

	status, body := sendFetch(t, env, http.MethodPost, env.server.URL+"/cart/items", otherOrigin, url.Values{
		"csrf_token": {token},
		"variant_id": {variantID},
		"qty":        {"1"},
	})
	if status != http.StatusForbidden {
		t.Fatalf("a fetch from another origin = %d, want 403", status)
	}
	if !strings.Contains(body, "Reload the page") {
		t.Errorf("the fragment does not say what to do: %s", body)
	}
	if strings.Contains(body, "</html>") {
		t.Errorf("a whole page was swapped into the cart node: %s", body)
	}
}

// The panel case that started this: an operator signed in, clicked a status and
// got a blank page. The answer keeps the admin chrome, because the session is
// fine and only the page is stale.
func TestARefusedPanelFormKeepsTheAdminChrome(t *testing.T) {
	env := startShopEnv(t)
	p := paidOrder(t, env)
	client := signIn(t, env)
	id := adminOrderID(t, env, client, p.number)

	_, card := get(t, client, env.server.URL+"/admin/orders/"+id)
	status, body := send(t, client, http.MethodPost, env.server.URL+"/admin/orders/"+id+"/status", otherOrigin, url.Values{
		"csrf_token": {capture(t, reCSRF, card, "csrf token")},
		"status":     {"shipped"},
	})
	if status != http.StatusForbidden {
		t.Fatalf("a panel form from another origin = %d, want 403", status)
	}
	if strings.TrimSpace(body) == "forbidden" {
		t.Fatal("the panel refusal is still the bare forbidden text")
	}
	for _, want := range []string{"This page has expired", "Reload the panel", "QZQ admin"} {
		if !strings.Contains(body, want) {
			t.Errorf("the panel refusal does not show %q: %s", want, body)
		}
	}
	if got := orderStatusOf(t, env, p.number); got != "paid" {
		t.Errorf("the refused move still changed the order to %q", got)
	}
}

// The refusal must stay a refusal: explaining it is not the same as letting it
// through, and a request that is in order still passes.
func TestTheOriginCheckStillRefusesAndStillAdmits(t *testing.T) {
	env := startShopEnv(t)

	_, home := get(t, env.client, env.server.URL+"/")
	token := capture(t, reCSRF, home, "csrf token")
	variantID := capture(t, reVariant, home, "variant id")
	form := url.Values{"csrf_token": {token}, "variant_id": {variantID}, "qty": {"1"}}

	if status, _ := send(t, env.client, http.MethodPost, env.server.URL+"/cart/items", env.server.URL, form); status != http.StatusOK {
		t.Errorf("a form from the shop itself = %d, want 200", status)
	}
	// A stale token from the right origin is refused just the same.
	stale := url.Values{"csrf_token": {"0000"}, "variant_id": {variantID}, "qty": {"1"}}
	if status, _ := send(t, env.client, http.MethodPost, env.server.URL+"/cart/items", env.server.URL, stale); status != http.StatusForbidden {
		t.Errorf("a stale token = %d, want 403", status)
	}
}

// sendFetch posts the way web/static/js/cart.js does: with the header that says
// the answer will be swapped into a node.
func sendFetch(t *testing.T, env *shopEnv, method, target, origin string, form url.Values) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), method, target, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", origin)
	req.Header.Set("X-Requested-With", "fetch")
	resp, err := env.client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, target, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, string(body)
}
