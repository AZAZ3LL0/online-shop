package httpx_test

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

var reAdminOrderLink = regexp.MustCompile(`/admin/orders/([0-9a-f-]{36})`)

// paidOrder walks a real checkout, links a chat to it and pays it through the
// provider callback, so the admin tests start from the state an operator
// actually opens: a paid order somebody is following in Telegram.
func paidOrder(t *testing.T, env *shopEnv) placed {
	t.Helper()
	p := checkout(t, env, "2")
	track(t, env, 8100, p.number)
	if status, body := callback(t, env, p.number, "finished", true); status != http.StatusOK {
		t.Fatalf("paying callback = %d: %s", status, body)
	}
	if got := orderStatusOf(t, env, p.number); got != "paid" {
		t.Fatalf("order is %q, want paid before the admin takes over", got)
	}
	return p
}

// adminOrderID finds one order in the list by its number, the way an operator
// does: through the filter bar.
func adminOrderID(t *testing.T, env *shopEnv, client *http.Client, number string) string {
	t.Helper()
	status, list := get(t, client, env.server.URL+"/admin/orders?number="+url.QueryEscape(number))
	if status != http.StatusOK {
		t.Fatalf("GET /admin/orders = %d", status)
	}
	if !strings.Contains(list, number) {
		t.Fatalf("the filtered list does not contain %s: %s", number, list)
	}
	return capture(t, reAdminOrderLink, list, "order link")
}

// postStatus asks for a manual transition and hands back the answer.
func postStatus(t *testing.T, env *shopEnv, client *http.Client, id, status string) (int, string) {
	t.Helper()
	_, card := get(t, client, env.server.URL+"/admin/orders/"+id)
	return send(t, client, http.MethodPost, env.server.URL+"/admin/orders/"+id+"/status", env.server.URL, url.Values{
		"csrf_token": {capture(t, reCSRF, card, "csrf token")},
		"status":     {status},
	})
}

// S5.2 acceptance: the list filters, and the filters are applied in SQL rather
// than by showing everything and hoping.
func TestAdminOrderListFilters(t *testing.T) {
	env := startShopEnv(t)
	p := paidOrder(t, env)
	client := signIn(t, env)

	status, unfiltered := get(t, client, env.server.URL+"/admin/orders")
	if status != http.StatusOK {
		t.Fatalf("GET /admin/orders = %d", status)
	}
	if !strings.Contains(unfiltered, p.number) {
		t.Fatalf("the list does not show the new order %s", p.number)
	}

	_, byStatus := get(t, client, env.server.URL+"/admin/orders?status=paid")
	if !strings.Contains(byStatus, p.number) {
		t.Error("filtering by paid hides a paid order")
	}
	_, byOtherStatus := get(t, client, env.server.URL+"/admin/orders?status=shipped")
	if strings.Contains(byOtherStatus, p.number) {
		t.Error("filtering by shipped shows a paid order")
	}
	_, byNumber := get(t, client, env.server.URL+"/admin/orders?number="+url.QueryEscape(p.number))
	if !strings.Contains(byNumber, p.number) {
		t.Error("searching by number hides the order")
	}
	_, byOtherNumber := get(t, client, env.server.URL+"/admin/orders?number=ORD-000000-0000")
	if strings.Contains(byOtherNumber, p.number) {
		t.Error("searching for another number still shows the order")
	}
	_, byFuturePeriod := get(t, client, env.server.URL+"/admin/orders?from=2099-01-01")
	if strings.Contains(byFuturePeriod, p.number) {
		t.Error("a period in the future still lists today's order")
	}
}

// The card is the operator's whole view of one order: the price snapshot, the
// money, the attribution, the provider log and the Telegram links (tech.md §8.4).
func TestAdminOrderCardShowsTheOrderBehindIt(t *testing.T) {
	env := startShopEnv(t)
	p := paidOrder(t, env)
	client := signIn(t, env)
	id := adminOrderID(t, env, client, p.number)

	status, card := get(t, client, env.server.URL+"/admin/orders/"+id)
	if status != http.StatusOK {
		t.Fatalf("GET the card = %d", status)
	}
	for _, want := range []string{
		p.number,
		"Samat Sadriev",     // the customer block
		"buyer@example.com", // the contact
		"$70.00",            // two units of the snapshot price
		"finished",          // the provider log
		`value="shipped"`,   // the manual transition on offer
	} {
		if !strings.Contains(card, want) {
			t.Errorf("the card does not show %q", want)
		}
	}
	// Cancelling is not a move a human owns from paid, tech.md §5.1.
	if strings.Contains(card, `value="cancelled"`) {
		t.Error("the card offers cancelling a paid order")
	}
	if !strings.Contains(card, "4242") && !strings.Contains(card, "@buyer") {
		t.Errorf("the card does not show the chat following the order: %s", card)
	}
}

// S5.2 acceptance: a manual move goes through CanTransition and queues exactly
// one message for the buyer.
func TestAdminManualTransitionQueuesOneMessage(t *testing.T) {
	env := startShopEnv(t)
	p := paidOrder(t, env)
	client := signIn(t, env)
	id := adminOrderID(t, env, client, p.number)

	if status, body := postStatus(t, env, client, id, "shipped"); status != http.StatusOK {
		t.Fatalf("moving to shipped = %d: %s", status, body)
	}
	if got := orderStatusOf(t, env, p.number); got != "shipped" {
		t.Fatalf("order is %q after the move, want shipped", got)
	}
	if status, body := postStatus(t, env, client, id, "delivered"); status != http.StatusOK {
		t.Fatalf("moving to delivered = %d: %s", status, body)
	}
	if got := orderStatusOf(t, env, p.number); got != "delivered" {
		t.Fatalf("order is %q after the move, want delivered", got)
	}

	texts := outboxTexts(t, env, p.number)
	if len(texts) != 3 {
		t.Fatalf("queued messages = %d (%v), want one for paid, shipped and delivered", len(texts), texts)
	}
	for _, want := range []string{"is paid", "has shipped", "has been delivered"} {
		if count := countTexts(texts, want); count != 1 {
			t.Errorf("%d messages mention %q, want exactly one", count, want)
		}
	}
}

// The error path: a move the status machine forbids changes nothing and queues
// nothing, whether it points backwards or sideways.
func TestAdminRejectsAForbiddenTransition(t *testing.T) {
	env := startShopEnv(t)
	p := paidOrder(t, env)
	client := signIn(t, env)
	id := adminOrderID(t, env, client, p.number)
	before := len(outboxTexts(t, env, p.number))

	for _, target := range []string{"awaiting_payment", "cancelled", "expired", "refunded", "nonsense"} {
		status, body := postStatus(t, env, client, id, target)
		if status != http.StatusConflict {
			t.Errorf("moving a paid order to %q = %d, want 409", target, status)
			continue
		}
		if !strings.Contains(body, "not allowed") {
			t.Errorf("the refusal of %q does not say why: %s", target, body)
		}
		if got := orderStatusOf(t, env, p.number); got != "paid" {
			t.Fatalf("the refused move to %q left the order in %q", target, got)
		}
	}
	if after := len(outboxTexts(t, env, p.number)); after != before {
		t.Errorf("refused moves queued %d messages", after-before)
	}
}

func countTexts(texts []string, needle string) int {
	var n int
	for _, text := range texts {
		if strings.Contains(text, needle) {
			n++
		}
	}
	return n
}
