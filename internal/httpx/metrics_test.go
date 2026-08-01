package httpx_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/qzq-kiim/shop/internal/auth"
	"github.com/qzq-kiim/shop/internal/storage/postgres"
)

// metricsPayload mirrors the response of GET /admin/api/metrics. The test
// declares the contract itself instead of importing the handler types, so a
// silent rename of a JSON field fails here.
type metricsPayload struct {
	Range struct {
		Period      string `json:"period"`
		Granularity string `json:"granularity"`
		From        string `json:"from"`
		To          string `json:"to"`
		Days        int    `json:"days"`
	} `json:"range"`
	Totals struct {
		Visits             int64     `json:"visits"`
		Visitors           int64     `json:"visitors"`
		Orders             int64     `json:"orders"`
		PaidOrders         int64     `json:"paid_orders"`
		Revenue            wireMoney `json:"revenue"`
		AverageOrder       wireMoney `json:"average_order"`
		Conversion         float64   `json:"conversion"`
		AbandonedCartShare float64   `json:"abandoned_cart_share"`
	} `json:"totals"`
	Series []struct {
		At         string    `json:"at"`
		Orders     int64     `json:"orders"`
		PaidOrders int64     `json:"paid_orders"`
		Revenue    wireMoney `json:"revenue"`
	} `json:"series"`
	PriceChanges []struct {
		ProductID    string    `json:"product_id"`
		ProductTitle string    `json:"product_title"`
		At           string    `json:"at"`
		OldPrice     wireMoney `json:"old_price"`
		NewPrice     wireMoney `json:"new_price"`
		Delta        wireMoney `json:"delta"`
		Reason       string    `json:"reason"`
	} `json:"price_changes"`
	Funnel []struct {
		Stage            string  `json:"stage"`
		Label            string  `json:"label"`
		Count            int64   `json:"count"`
		RateFromPrevious float64 `json:"rate_from_previous"`
		RateFromTop      float64 `json:"rate_from_top"`
	} `json:"funnel"`
	Sources []struct {
		Source     string    `json:"source"`
		Medium     string    `json:"medium"`
		Campaign   string    `json:"campaign"`
		Referrer   string    `json:"referrer"`
		Label      string    `json:"label"`
		Visitors   int64     `json:"visitors"`
		Orders     int64     `json:"orders"`
		Revenue    wireMoney `json:"revenue"`
		Conversion float64   `json:"conversion"`
	} `json:"sources"`
}

// wireMoney is how money crosses the wire: minor units plus the rendered form,
// never a float (tech.md §6.1).
type wireMoney struct {
	Cents    int64  `json:"cents"`
	Currency string `json:"currency"`
	Display  string `json:"display"`
}

// TestMetricsEndpointIsClosedWithoutASession keeps the report behind the same
// guard as the rest of the admin panel, tech.md §8.5.
func TestMetricsEndpointIsClosedWithoutASession(t *testing.T) {
	env := startShopEnv(t)
	client := newClient(t)
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	status, _ := get(t, client, env.server.URL+"/admin/api/metrics")
	if status != http.StatusSeeOther {
		t.Fatalf("GET /admin/api/metrics without a session = %d, want 303", status)
	}
}

// TestMetricsShapePerGranularity is the S6.2 acceptance criteria: the endpoint
// answers the same documented shape for every granularity, and the buckets it
// returns match the one that was asked for.
func TestMetricsShapePerGranularity(t *testing.T) {
	env := startShopEnv(t)
	client := signInAdmin(t, env)

	cases := []struct {
		name        string
		query       string
		granularity string
		minBuckets  int
		maxBuckets  int
	}{
		{"today defaults to hourly", "?range=today", "hour", 24, 24},
		{"seven days", "?range=7d&granularity=day", "day", 7, 7},
		{"thirty days", "?range=30d&granularity=day", "day", 30, 30},
		{"thirty days by week", "?range=30d&granularity=week", "week", 5, 6},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report := fetchMetrics(t, client, env.server.URL+"/admin/api/metrics"+tc.query)

			if report.Range.Granularity != tc.granularity {
				t.Fatalf("granularity = %q, want %q", report.Range.Granularity, tc.granularity)
			}
			if len(report.Series) < tc.minBuckets || len(report.Series) > tc.maxBuckets {
				t.Fatalf("series = %d buckets, want %d..%d", len(report.Series), tc.minBuckets, tc.maxBuckets)
			}
			if report.Range.From == "" || report.Range.To == "" {
				t.Fatal("the range bounds are not reported")
			}
			if len(report.Funnel) != 5 {
				t.Fatalf("funnel = %d stages, want 5", len(report.Funnel))
			}
			for _, amount := range []wireMoney{report.Totals.Revenue, report.Totals.AverageOrder} {
				if amount.Currency != "USD" || !strings.HasPrefix(amount.Display, "$") {
					t.Fatalf("money is not reported as minor units and a rendered string: %+v", amount)
				}
			}
			// Every bucket carries its own money, so the chart never divides.
			for _, bucket := range report.Series {
				if bucket.Revenue.Currency != "USD" {
					t.Fatalf("bucket %s has no currency", bucket.At)
				}
			}
		})
	}
}

// TestMetricsReportsSeededRevenueAndPriceMarkers checks the aggregate itself
// against the demo dataset: the money the endpoint reports is the money in the
// orders table, and the markers come from price_history (tech.md §8.2).
func TestMetricsReportsSeededRevenueAndPriceMarkers(t *testing.T) {
	env := startShopEnv(t)
	client := signInAdmin(t, env)

	report := fetchMetrics(t, client, env.server.URL+"/admin/api/metrics?range=30d&granularity=day")

	var (
		paidOrders int64
		revenue    int64
	)
	err := env.store.Pool().QueryRow(context.Background(), `
		SELECT count(*), coalesce(sum(total_cents), 0)
		FROM orders
		WHERE status IN ('paid', 'shipped', 'delivered')
		  AND created_at >= now() - interval '30 days'`).Scan(&paidOrders, &revenue)
	if err != nil {
		t.Fatalf("read seeded orders: %v", err)
	}
	if paidOrders == 0 {
		t.Fatal("the demo dataset has no paid orders to report on")
	}
	if report.Totals.PaidOrders != paidOrders {
		t.Errorf("paid orders = %d, want %d", report.Totals.PaidOrders, paidOrders)
	}
	if report.Totals.Revenue.Cents != revenue {
		t.Errorf("revenue = %d cents, want %d", report.Totals.Revenue.Cents, revenue)
	}
	if report.Totals.AverageOrder.Cents != revenue/paidOrders {
		t.Errorf("average order = %d cents, want %d", report.Totals.AverageOrder.Cents, revenue/paidOrders)
	}

	// The sum of the buckets is the total: the chart and the card cannot drift.
	var bucketed int64
	for _, bucket := range report.Series {
		bucketed += bucket.Revenue.Cents
	}
	if bucketed != report.Totals.Revenue.Cents {
		t.Errorf("buckets sum to %d cents, totals report %d", bucketed, report.Totals.Revenue.Cents)
	}

	// The seeder writes one price change per product, fourteen days back.
	if len(report.PriceChanges) != 3 {
		t.Fatalf("price markers = %d, want one per seeded product", len(report.PriceChanges))
	}
	for _, change := range report.PriceChanges {
		if change.ProductTitle == "" {
			t.Error("a price marker does not name its product")
		}
		if change.Delta.Cents != change.NewPrice.Cents-change.OldPrice.Cents {
			t.Errorf("marker delta %d does not match %d -> %d",
				change.Delta.Cents, change.OldPrice.Cents, change.NewPrice.Cents)
		}
	}

	// A window that predates the demo data reports zeros, not an error.
	empty := fetchMetrics(t, client,
		env.server.URL+"/admin/api/metrics?range=custom&from=2020-01-01&to=2020-01-07")
	if empty.Totals.PaidOrders != 0 || empty.Totals.Revenue.Cents != 0 {
		t.Errorf("an empty window reports %+v", empty.Totals)
	}
	if len(empty.Series) != 7 {
		t.Errorf("an empty window returns %d buckets, want 7 zeroed ones", len(empty.Series))
	}
}

// TestFunnelFallsOffMonotonically is the S6.4 acceptance criteria: on the demo
// dataset each step of the funnel is a subset of the one before it.
func TestFunnelFallsOffMonotonically(t *testing.T) {
	env := startShopEnv(t)
	client := signInAdmin(t, env)

	report := fetchMetrics(t, client, env.server.URL+"/admin/api/metrics?range=30d&granularity=day")

	wantStages := []string{"visits", "product_views", "cart_adds", "checkouts", "payments"}
	if len(report.Funnel) != len(wantStages) {
		t.Fatalf("funnel = %d stages, want %d", len(report.Funnel), len(wantStages))
	}
	for i, stage := range report.Funnel {
		if stage.Stage != wantStages[i] {
			t.Fatalf("stage %d = %q, want %q", i, stage.Stage, wantStages[i])
		}
		if i > 0 && stage.Count > report.Funnel[i-1].Count {
			t.Fatalf("stage %q counts %d, more than %q with %d",
				stage.Stage, stage.Count, report.Funnel[i-1].Stage, report.Funnel[i-1].Count)
		}
		if stage.RateFromPrevious > 1.0001 || stage.RateFromTop > 1.0001 {
			t.Fatalf("stage %q reports a rate above one: %+v", stage.Stage, stage)
		}
	}
	if report.Funnel[0].Count == 0 {
		t.Fatal("the demo dataset produced no visits to open the funnel with")
	}
	if report.Funnel[len(report.Funnel)-1].Count == 0 {
		t.Fatal("the demo dataset produced no payments to close the funnel with")
	}

	// Traffic sources: the seeded campaigns are reported with their money.
	if len(report.Sources) == 0 {
		t.Fatal("no traffic sources were reported")
	}
	var (
		seenCampaign bool
		seenDirect   bool
		sourceOrders int64
	)
	for _, source := range report.Sources {
		if source.Source == "instagram" && source.Medium == "social" {
			seenCampaign = true
			if source.Visitors == 0 {
				t.Error("a campaign row reports no visitors")
			}
		}
		if source.Label == "direct" {
			seenDirect = true
		}
		sourceOrders += source.Orders
	}
	if !seenCampaign {
		t.Error("the seeded instagram campaign is missing from the traffic table")
	}
	if !seenDirect {
		t.Error("untagged traffic is not reported as direct")
	}
	if sourceOrders != report.Totals.PaidOrders {
		t.Errorf("traffic table credits %d orders, totals report %d", sourceOrders, report.Totals.PaidOrders)
	}
}

// fetchMetrics reads the report as an authenticated administrator.
func fetchMetrics(t *testing.T, client *http.Client, target string) metricsPayload {
	t.Helper()
	status, body := get(t, client, target)
	if status != http.StatusOK {
		t.Fatalf("GET %s = %d, body: %s", target, status, body)
	}
	var payload metricsPayload
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("decode metrics: %v, body: %s", err, body)
	}
	return payload
}

// signInAdmin creates an administrator and returns a client holding its session.
func signInAdmin(t *testing.T, env *shopEnv) *http.Client {
	t.Helper()
	const login = "metrics-admin"
	// The password never leaves the test and protects nothing.
	const password = "correct horse battery staple"

	hash, err := auth.Hash(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if _, err := postgres.NewAdminRepo(env.store).Upsert(context.Background(), login, hash); err != nil {
		t.Fatalf("create admin: %v", err)
	}

	client := newClient(t)
	_, form := get(t, client, env.server.URL+"/admin/login")
	token := capture(t, reCSRF, form, "csrf token")

	status, body := send(t, client, http.MethodPost, env.server.URL+"/admin/login", env.server.URL, url.Values{
		"csrf_token": {token},
		"login":      {login},
		"password":   {password},
	})
	if status != http.StatusOK {
		t.Fatalf("admin login = %d, body: %s", status, body)
	}
	return client
}
