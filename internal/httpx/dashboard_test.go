package httpx_test

import (
	"net/http"
	"strings"
	"testing"
)

// TestDashboardRendersTheReport is the S6.3 acceptance criteria: the dashboard
// serves the cards, the chart canvas and the price markers of the period.
func TestDashboardRendersTheReport(t *testing.T) {
	env := startShopEnv(t)
	client := signInAdmin(t, env)

	status, body := get(t, client, env.server.URL+"/admin?range=30d")
	if status != http.StatusOK {
		t.Fatalf("GET /admin = %d", status)
	}
	for _, card := range []string{"Revenue", "Orders", "Average order", "Conversion", "Abandoned carts"} {
		if !strings.Contains(body, card) {
			t.Errorf("the dashboard does not show the %q card", card)
		}
	}
	if !strings.Contains(body, `id="revenue-chart"`) {
		t.Error("the revenue chart canvas is missing")
	}
	if !strings.Contains(body, `data-metrics-url="/admin/api/metrics?`) {
		t.Error("the charts are not pointed at the metrics endpoint")
	}
	// The markers are the price changes of the period, tech.md §8.2.
	if !strings.Contains(body, "data-price-marker=") {
		t.Error("the dashboard shows no price change markers")
	}
	if !strings.Contains(body, "launch price correction") {
		t.Error("a price marker does not carry the reason it was changed for")
	}
	// The seeded revenue is rendered as money, never as a bare cent count.
	report := fetchMetrics(t, client, env.server.URL+"/admin/api/metrics?range=30d&granularity=day")
	if !strings.Contains(body, report.Totals.Revenue.Display) {
		t.Errorf("the revenue card does not show %s", report.Totals.Revenue.Display)
	}

	// A broken selector falls back to the default period instead of erroring.
	status, _ = get(t, client, env.server.URL+"/admin?range=nonsense")
	if status != http.StatusOK {
		t.Fatalf("GET /admin with a broken range = %d, want the default period", status)
	}
}

// TestAnalyticsPageRendersFunnelAndSources is the page half of S6.4.
func TestAnalyticsPageRendersFunnelAndSources(t *testing.T) {
	env := startShopEnv(t)
	client := signInAdmin(t, env)

	status, body := get(t, client, env.server.URL+"/admin/analytics?range=30d")
	if status != http.StatusOK {
		t.Fatalf("GET /admin/analytics = %d", status)
	}
	for _, stage := range []string{"Visits", "Product views", "Added to cart", "Checkouts", "Paid"} {
		if !strings.Contains(body, stage) {
			t.Errorf("the funnel does not show the %q stage", stage)
		}
	}
	if !strings.Contains(body, `id="funnel-chart"`) {
		t.Error("the funnel chart canvas is missing")
	}
	for _, column := range []string{"Source", "Visitors", "Orders", "Revenue", "Conversion"} {
		if !strings.Contains(body, column) {
			t.Errorf("the traffic table has no %q column", column)
		}
	}
	if !strings.Contains(body, "instagram / social / drop-1") {
		t.Error("the traffic table does not name the seeded campaign")
	}
}
