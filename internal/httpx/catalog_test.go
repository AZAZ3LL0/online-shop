package httpx_test

import (
	"net/http"
	"strings"
	"testing"
)

// seedTitles is the demo collection in sort_order, see postgres.Seeder.
var seedTitles = []string{"QZQ Black", "QZQ White", "Besh"}

// TestHomeListsActiveProductsInSortOrder is the S1.1 acceptance criteria: the
// storefront index shows the three active models in sort_order, each with its
// price, its size selector and a link to its product page.
func TestHomeListsActiveProductsInSortOrder(t *testing.T) {
	server, client := startShop(t)

	status, body := get(t, client, server.URL+"/")
	if status != http.StatusOK {
		t.Fatalf("GET / = %d", status)
	}

	positions := make([]int, len(seedTitles))
	for i, title := range seedTitles {
		positions[i] = strings.Index(body, title)
		if positions[i] < 0 {
			t.Fatalf("home page does not show %q", title)
		}
	}
	for i := 1; i < len(positions); i++ {
		if positions[i] < positions[i-1] {
			t.Fatalf("products are out of sort_order: %q renders before %q", seedTitles[i], seedTitles[i-1])
		}
	}

	if got, want := strings.Count(body, `name="variant_id"`), len(seedTitles); got != want {
		t.Fatalf("size selectors on the home page = %d, want one per product (%d)", got, want)
	}
	for _, price := range []string{"$35.00", "$39.00"} {
		if !strings.Contains(body, price) {
			t.Fatalf("home page does not show the price %s", price)
		}
	}
	for _, slug := range []string{"qzq-black", "qzq-white", "besh"} {
		if !strings.Contains(body, `href="/product/`+slug+`"`) {
			t.Fatalf("card for %q does not link to its product page", slug)
		}
	}
}

// TestHomeShowsBothCoversForEveryCard covers the flip requirement at the level
// that is worth testing: both covers must reach the browser, so the CSS hover
// and the Alpine tap have something to switch between. The visual transition
// itself is markup and is not tested, tech.md §11.
func TestHomeShowsBothCoversForEveryCard(t *testing.T) {
	server, client := startShop(t)

	_, body := get(t, client, server.URL+"/")
	for _, img := range []string{
		"/static/img/qzqblackfront-removebg-preview.png",
		"/static/img/qzqblackback-removebg-preview.png",
		"/static/img/qzqwhitefront-removebg-preview.png",
		"/static/img/qzqwhiteback-removebg-preview.png",
		"/static/img/beshfront-removebg-preview.png",
		"/static/img/beshback-removebg-preview.png",
	} {
		if !strings.Contains(body, img) {
			t.Fatalf("home page does not reference %s", img)
		}
	}
}
