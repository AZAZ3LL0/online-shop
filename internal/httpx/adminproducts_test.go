package httpx_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// productPrice reads the live catalogue price straight from the database.
func productPrice(t *testing.T, env *shopEnv, productID string) int64 {
	t.Helper()
	var cents int64
	err := env.store.Pool().QueryRow(context.Background(),
		`SELECT price_cents FROM products WHERE id = $1`, productID).Scan(&cents)
	if err != nil {
		t.Fatalf("read product price: %v", err)
	}
	return cents
}

// orderedProduct returns the product of an order's only line, together with the
// price snapshot the order carries.
func orderedProduct(t *testing.T, env *shopEnv, number string) (string, int64) {
	t.Helper()
	var productID string
	var snapshot int64
	err := env.store.Pool().QueryRow(context.Background(),
		`SELECT v.product_id, i.unit_price_cents
		 FROM order_items i
		 JOIN product_variants v ON v.id = i.variant_id
		 JOIN orders o ON o.id = i.order_id
		 WHERE o.number = $1`, number).Scan(&productID, &snapshot)
	if err != nil {
		t.Fatalf("read ordered product: %v", err)
	}
	return productID, snapshot
}

// priceHistory returns the recorded price changes of one product, newest first.
func priceHistory(t *testing.T, env *shopEnv, productID string) []priceChange {
	t.Helper()
	rows, err := env.store.Pool().Query(context.Background(),
		`SELECT h.old_price_cents, h.new_price_cents, coalesce(h.reason, ''), coalesce(u.login, '')
		 FROM price_history h
		 LEFT JOIN admin_users u ON u.id = h.changed_by
		 WHERE h.product_id = $1
		 ORDER BY h.created_at DESC`, productID)
	if err != nil {
		t.Fatalf("read price history: %v", err)
	}
	defer rows.Close()

	var out []priceChange
	for rows.Next() {
		var c priceChange
		if err := rows.Scan(&c.oldCents, &c.newCents, &c.reason, &c.changedBy); err != nil {
			t.Fatalf("scan price history: %v", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate price history: %v", err)
	}
	return out
}

type priceChange struct {
	oldCents  int64
	newCents  int64
	reason    string
	changedBy string
}

// postPrice submits the inline price editor of one model.
func postPrice(t *testing.T, env *shopEnv, client *http.Client, productID, price, reason string) (int, string) {
	t.Helper()
	_, page := get(t, client, env.server.URL+"/admin/products")
	return send(t, client, http.MethodPost, env.server.URL+"/admin/products/"+productID+"/price", env.server.URL, url.Values{
		"csrf_token": {capture(t, reCSRF, page, "csrf token")},
		"price":      {price},
		"reason":     {reason},
	})
}

// postStock submits the stock editor of one size.
func postStock(t *testing.T, env *shopEnv, client *http.Client, productID, variantID, stock string) (int, string) {
	t.Helper()
	_, page := get(t, client, env.server.URL+"/admin/products")
	return send(t, client, http.MethodPost, env.server.URL+"/admin/products/"+productID+"/stock", env.server.URL, url.Values{
		"csrf_token": {capture(t, reCSRF, page, "csrf token")},
		"variant_id": {variantID},
		"stock":      {stock},
	})
}

// S5.3 acceptance: a price change needs a reason, lands in price_history with
// the administrator who made it, and leaves every order already placed alone -
// those carry their own snapshot in order_items (tech.md §8.3).
func TestAdminPriceChangeIsRecordedAndLeavesOrdersAlone(t *testing.T) {
	env := startShopEnv(t)
	p := paidOrder(t, env)
	productID, snapshot := orderedProduct(t, env, p.number)
	before := productPrice(t, env, productID)
	trailBefore := len(priceHistory(t, env, productID))
	client := signIn(t, env)

	// Without a reason nothing moves.
	status, body := postPrice(t, env, client, productID, "99.50", "  ")
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("a price change without a reason = %d, want 422", status)
	}
	if !strings.Contains(body, "Say why the price changes") {
		t.Errorf("the refusal does not ask for a reason: %s", body)
	}
	if got := productPrice(t, env, productID); got != before {
		t.Fatalf("the refused change moved the price to %d", got)
	}
	if got := len(priceHistory(t, env, productID)); got != trailBefore {
		t.Fatalf("the refused change wrote %d history rows", got-trailBefore)
	}

	// With one, the price moves and the change is on the record.
	if status, body := postPrice(t, env, client, productID, "99.50", "autumn drop repricing"); status != http.StatusOK {
		t.Fatalf("a valid price change = %d: %s", status, body)
	}
	if got := productPrice(t, env, productID); got != 9950 {
		t.Fatalf("price is %d cents, want 9950", got)
	}
	trail := priceHistory(t, env, productID)
	if len(trail) != trailBefore+1 {
		t.Fatalf("history rows = %d, want one more than %d", len(trail), trailBefore)
	}
	latest := trail[0]
	if latest.oldCents != before || latest.newCents != 9950 {
		t.Errorf("history row records %d -> %d, want %d -> 9950", latest.oldCents, latest.newCents, before)
	}
	if latest.reason != "autumn drop repricing" {
		t.Errorf("history row reason = %q", latest.reason)
	}
	if latest.changedBy != testAdminLogin {
		t.Errorf("history row was written by %q, want %q", latest.changedBy, testAdminLogin)
	}

	// The invariant of the slice: the placed order keeps the price it was sold at.
	if _, after := orderedProduct(t, env, p.number); after != snapshot {
		t.Fatalf("the order line moved from %d to %d cents", snapshot, after)
	}
}

// S5.3 acceptance: stock is editable per size, but never below the units a live
// checkout has already reserved.
func TestAdminStockEditRespectsTheReservation(t *testing.T) {
	env := startShopEnv(t)
	// An unpaid checkout keeps two units reserved on the size it took.
	p := checkout(t, env, "2")
	_, reserved := variantStock(t, env, p.variantID)
	if reserved != 2 {
		t.Fatalf("reserved = %d, want the two units of the open checkout", reserved)
	}
	client := signIn(t, env)

	status, page := get(t, client, env.server.URL+"/admin/products")
	if status != http.StatusOK {
		t.Fatalf("GET /admin/products = %d", status)
	}
	productID, _ := orderedProduct(t, env, p.number)
	if !strings.Contains(page, "product-"+productID) {
		t.Fatalf("the product page does not show the model behind the order")
	}
	if !strings.Contains(page, `name="variant_id" value="`+p.variantID+`"`) {
		t.Fatalf("the product page has no stock form for the reserved size")
	}

	// One unit would leave the two reserved ones uncovered.
	status, body := postStock(t, env, client, productID, p.variantID, "1")
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("cutting the stock below the reservation = %d, want 422", status)
	}
	if !strings.Contains(body, "reserved in a live checkout") {
		t.Errorf("the refusal does not explain the reservation: %s", body)
	}

	// A number that covers them is accepted.
	if status, body := postStock(t, env, client, productID, p.variantID, "40"); status != http.StatusOK {
		t.Fatalf("raising the stock = %d: %s", status, body)
	}
	stock, stillReserved := variantStock(t, env, p.variantID)
	if stock != 40 || stillReserved != 2 {
		t.Fatalf("stock/reserved = %d/%d, want 40/2", stock, stillReserved)
	}

	// A negative number is refused before it reaches the database.
	if status, _ := postStock(t, env, client, productID, p.variantID, "-1"); status != http.StatusUnprocessableEntity {
		t.Errorf("a negative stock = %d, want 422", status)
	}
	if stock, _ := variantStock(t, env, p.variantID); stock != 40 {
		t.Errorf("the refused edit left the stock at %d", stock)
	}
}
