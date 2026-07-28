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

// TestProductPageShowsTheSizeRun is the S1.2 acceptance criteria. The white tee
// is seeded with an empty XXL, so one page carries both the buyable and the
// sold out case.
func TestProductPageShowsTheSizeRun(t *testing.T) {
	server, client := startShop(t)

	status, body := get(t, client, server.URL+"/product/qzq-white")
	if status != http.StatusOK {
		t.Fatalf("GET /product/qzq-white = %d", status)
	}
	if !strings.Contains(body, "QZQ White") {
		t.Fatal("product page does not show the title")
	}
	if !strings.Contains(body, "off-white, oversized fit") {
		t.Fatal("product page does not show the description")
	}
	if !strings.Contains(body, "$35.00") {
		t.Fatal("product page does not show the price")
	}
	for _, size := range []string{"S", "M", "L", "XL", "XXL"} {
		if !strings.Contains(body, ">"+size+"</td>") {
			t.Fatalf("size run is missing %q", size)
		}
	}

	// XXL is seeded with zero stock: it must be flagged and unselectable.
	if !strings.Contains(body, "sold out") {
		t.Fatal("the empty size is not flagged as sold out")
	}
	if !strings.Contains(body, "XXL - sold out") {
		t.Fatalf("the sold out size is still offered as a plain option: %s", body)
	}
	if !strings.Contains(body, "in stock") {
		t.Fatal("the available sizes are not flagged as in stock")
	}
	if !strings.Contains(body, `name="variant_id"`) {
		t.Fatal("a product with stock left must offer the add form")
	}
}

// TestProductPageUnknownSlugIsNotFound checks the shared error handler: an
// unknown slug answers 404 with the neutral page, not with a stack trace and
// not with a buyable form.
func TestProductPageUnknownSlugIsNotFound(t *testing.T) {
	server, client := startShop(t)

	status, body := get(t, client, server.URL+"/product/there-is-no-such-tee")
	if status != http.StatusNotFound {
		t.Fatalf("GET an unknown product = %d, want 404", status)
	}
	if !strings.Contains(body, "Not found") {
		t.Fatalf("404 does not render the shared error page: %s", body)
	}
	if strings.Contains(body, `name="variant_id"`) {
		t.Fatal("the 404 page must not offer an add form")
	}
	if strings.Contains(body, "there-is-no-such-tee") {
		t.Fatal("the 404 page must not echo the requested slug")
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
