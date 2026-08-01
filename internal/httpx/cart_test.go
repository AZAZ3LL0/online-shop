package httpx_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/qzq-kiim/shop/internal/httpx/handler/shop"
)

// cookieValue returns the raw value of a cookie the server has set on the jar.
func cookieValue(t *testing.T, client *http.Client, serverURL, name string) (string, bool) {
	t.Helper()
	u, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	for _, c := range client.Jar.Cookies(u) {
		if c.Name == name {
			return c.Value, true
		}
	}
	return "", false
}

// addToCart puts one unit of the first size on the home page into the cart and
// returns the csrf token the following mutations reuse.
func addToCart(t *testing.T, client *http.Client, serverURL string, qty string) (token string, status int, fragment string) {
	t.Helper()
	_, home := get(t, client, serverURL+"/")
	token = capture(t, reCSRF, home, "csrf token")
	variantID := capture(t, reVariant, home, "variant id")

	status, fragment = send(t, client, http.MethodPost, serverURL+"/cart/items", serverURL, url.Values{
		"csrf_token": {token},
		"variant_id": {variantID},
		"qty":        {qty},
	})
	return token, status, fragment
}

// TestCartOpensOnTheFirstAdd is the S2.1 acceptance criteria on the cookie: a
// visitor who only browses gets no cart, and the first add issues one signed
// cart id that every later mutation reuses.
func TestCartOpensOnTheFirstAdd(t *testing.T) {
	server, client := startShop(t)

	for _, path := range []string{"/", "/product/qzq-black", "/cart"} {
		if status, _ := get(t, client, server.URL+path); status != http.StatusOK {
			t.Fatalf("GET %s = %d", path, status)
		}
		if _, ok := cookieValue(t, client, server.URL, shop.CookieCart); ok {
			t.Fatalf("GET %s opened a cart before anything was added", path)
		}
	}

	token, status, fragment := addToCart(t, client, server.URL, "1")
	if status != http.StatusOK {
		t.Fatalf("POST /cart/items = %d, body: %s", status, fragment)
	}
	first, ok := cookieValue(t, client, server.URL, shop.CookieCart)
	if !ok {
		t.Fatal("the first add did not issue a cart cookie")
	}
	if !strings.Contains(first, ".") {
		t.Fatalf("the cart cookie is not signed: %q", first)
	}
	if strings.Contains(first, uuid.Nil.String()) {
		t.Fatal("the cart cookie carries a raw, unsigned id")
	}

	// A second add stays in the same cart.
	_, item := cartLine(t, client, server.URL)
	status, _ = send(t, client, http.MethodPatch, server.URL+"/cart/items/"+item, server.URL, url.Values{
		"csrf_token": {token},
		"qty":        {"2"},
	})
	if status != http.StatusOK {
		t.Fatalf("PATCH = %d", status)
	}
	second, _ := cookieValue(t, client, server.URL, shop.CookieCart)
	if second != first {
		t.Fatal("a mutation replaced the cart of an existing visitor")
	}
}

// cartLine returns the cart page and the id of its single line.
func cartLine(t *testing.T, client *http.Client, serverURL string) (string, string) {
	t.Helper()
	status, page := get(t, client, serverURL+"/cart")
	if status != http.StatusOK {
		t.Fatalf("GET /cart = %d", status)
	}
	return page, capture(t, reItem, page, "cart item id")
}

// TestCartRejectsQuantitiesOutsideTheLimit is the S2.1 error path: 1..10 is the
// only quantity a line can hold, on the add route and on the update route, and
// a rejected request leaves the cart untouched.
func TestCartRejectsQuantitiesOutsideTheLimit(t *testing.T) {
	server, client := startShop(t)

	token, status, _ := addToCart(t, client, server.URL, "1")
	if status != http.StatusOK {
		t.Fatalf("first add = %d", status)
	}
	_, itemID := cartLine(t, client, server.URL)

	for _, qty := range []string{"0", "11", "-1", "1000", "two", ""} {
		t.Run("update qty="+qty, func(t *testing.T) {
			status, fragment := send(t, client, http.MethodPatch, server.URL+"/cart/items/"+itemID, server.URL, url.Values{
				"csrf_token": {token},
				"qty":        {qty},
			})
			if status != http.StatusUnprocessableEntity {
				t.Fatalf("PATCH qty=%q = %d, want 422", qty, status)
			}
			if !strings.Contains(fragment, "between 1 and 10") {
				t.Fatalf("PATCH qty=%q does not explain the limit: %s", qty, fragment)
			}
			if strings.Contains(fragment, "<html") {
				t.Fatal("a rejected mutation must still answer with the fragment")
			}
			page, _ := cartLine(t, client, server.URL)
			if !strings.Contains(page, `value="1"`) {
				t.Fatalf("PATCH qty=%q changed the stored quantity", qty)
			}
		})
	}

	// The limit is on the line, not on the request: one unit is already in the
	// cart, so ten more cross it.
	_, home := get(t, client, server.URL+"/")
	variantID := capture(t, reVariant, home, "variant id")
	status, fragment := send(t, client, http.MethodPost, server.URL+"/cart/items", server.URL, url.Values{
		"csrf_token": {token},
		"variant_id": {variantID},
		"qty":        {"10"},
	})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("adding past the line limit = %d, want 422", status)
	}
	if !strings.Contains(fragment, "between 1 and 10") {
		t.Fatalf("adding past the line limit does not explain why: %s", fragment)
	}
}

// TestRepeatedUpdateIsIdempotent pins the S2.1 idempotency criteria: sending
// the same quantity again is a no-op, not a second line and not a second unit.
func TestRepeatedUpdateIsIdempotent(t *testing.T) {
	server, client := startShop(t)

	token, status, _ := addToCart(t, client, server.URL, "2")
	if status != http.StatusOK {
		t.Fatalf("add = %d", status)
	}
	_, itemID := cartLine(t, client, server.URL)

	for attempt := 1; attempt <= 3; attempt++ {
		status, fragment := send(t, client, http.MethodPatch, server.URL+"/cart/items/"+itemID, server.URL, url.Values{
			"csrf_token": {token},
			"qty":        {"3"},
		})
		if status != http.StatusOK {
			t.Fatalf("PATCH attempt %d = %d, body: %s", attempt, status, fragment)
		}

		page, sameItem := cartLine(t, client, server.URL)
		if sameItem != itemID {
			t.Fatalf("attempt %d replaced the line %s with %s", attempt, itemID, sameItem)
		}
		if got := strings.Count(page, `name="qty"`); got != 1 {
			t.Fatalf("attempt %d left %d lines in the cart, want 1", attempt, got)
		}
		if !strings.Contains(page, `value="3"`) {
			t.Fatalf("attempt %d did not leave the quantity at 3", attempt)
		}
		// Three units at $35.00.
		if !strings.Contains(page, "$105.00") {
			t.Fatalf("attempt %d did not price the line at $105.00: %s", attempt, page)
		}
	}
}

// TestCartMutationsOnUnknownLinesAreNeutral covers the remaining error paths of
// the three routes: an id from another cart, a malformed id and a request from
// a visitor who has no cart at all all get the same answer, and none of them
// opens a cart.
func TestCartMutationsOnUnknownLinesAreNeutral(t *testing.T) {
	server, client := startShop(t)

	token, status, _ := addToCart(t, client, server.URL, "1")
	if status != http.StatusOK {
		t.Fatalf("add = %d", status)
	}

	for _, id := range []string{uuid.NewString(), "not-a-uuid"} {
		for _, method := range []string{http.MethodPatch, http.MethodDelete} {
			status, fragment := send(t, client, method, server.URL+"/cart/items/"+id, server.URL, url.Values{
				"csrf_token": {token},
				"qty":        {"2"},
			})
			if status != http.StatusNotFound {
				t.Fatalf("%s /cart/items/%s = %d, want 404", method, id, status)
			}
			if !strings.Contains(fragment, "no longer in your cart") {
				t.Fatalf("%s /cart/items/%s says %q", method, id, fragment)
			}
		}
	}

	// The line the visitor does own is still there and still priced.
	page, _ := cartLine(t, client, server.URL)
	if !strings.Contains(page, "$35.00") {
		t.Fatalf("the owned line did not survive the rejected mutations: %s", page)
	}

	// A visitor without a cart cannot be given one by a failed mutation.
	stranger := newClient(t)
	_, home := get(t, stranger, server.URL+"/")
	strangerToken := capture(t, reCSRF, home, "csrf token")
	status, _ = send(t, stranger, http.MethodDelete, server.URL+"/cart/items/"+uuid.NewString(), server.URL, url.Values{
		"csrf_token": {strangerToken},
	})
	if status != http.StatusNotFound {
		t.Fatalf("delete without a cart = %d, want 404", status)
	}
	if _, ok := cookieValue(t, stranger, server.URL, shop.CookieCart); ok {
		t.Fatal("a failed delete opened a cart")
	}
}
