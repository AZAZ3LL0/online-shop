package httpx_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/qzq-kiim/shop/internal/worker"
)

// expire runs one pass of the reservation worker over the real database.
func expire(t *testing.T, env *shopEnv) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := worker.NewExpirer(env.orders, log, time.Minute).Tick(context.Background()); err != nil {
		t.Fatalf("expiry pass: %v", err)
	}
}

// overdue backdates the reservation deadline of one order.
func overdue(t *testing.T, env *shopEnv, number string) {
	t.Helper()
	_, err := env.store.Pool().Exec(context.Background(),
		`UPDATE orders SET expires_at = now() - interval '1 minute' WHERE number = $1`, number)
	if err != nil {
		t.Fatalf("backdate order: %v", err)
	}
}

// S3.5: an order that was never paid for gives its units back, and a second
// pass over the same rows changes nothing.
func TestExpiryWorkerReleasesOverdueReservations(t *testing.T) {
	env := startShopEnv(t)
	order := checkout(t, env, "2")
	stockBefore, reservedBefore := variantStock(t, env, order.variantID)

	// Nothing is due yet.
	expire(t, env)
	if got := orderStatusOf(t, env, order.number); got != "awaiting_payment" {
		t.Fatalf("status = %q, want the reservation to still hold", got)
	}
	if _, reserved := variantStock(t, env, order.variantID); reserved != reservedBefore {
		t.Fatalf("reserved = %d, want %d untouched", reserved, reservedBefore)
	}

	overdue(t, env, order.number)
	expire(t, env)

	if got := orderStatusOf(t, env, order.number); got != "expired" {
		t.Fatalf("status = %q, want expired", got)
	}
	stock, reserved := variantStock(t, env, order.variantID)
	if stock != stockBefore {
		t.Errorf("stock = %d, want %d: an unpaid order never sells anything", stock, stockBefore)
	}
	if reserved != reservedBefore-2 {
		t.Errorf("reserved = %d, want %d", reserved, reservedBefore-2)
	}

	// A second pass must not release the same units twice.
	expire(t, env)
	stockAgain, reservedAgain := variantStock(t, env, order.variantID)
	if stockAgain != stock || reservedAgain != reserved {
		t.Errorf("second pass moved the stock: %d/%d, want %d/%d",
			stockAgain, reservedAgain, stock, reserved)
	}
	assertStockInvariant(t, env)
}

// A paid order is not the worker's business, whatever its deadline says.
func TestExpiryWorkerLeavesPaidOrdersAlone(t *testing.T) {
	env := startShopEnv(t)
	order := checkout(t, env, "1")
	if status, body := callback(t, env, order.number, "finished", true); status != 200 {
		t.Fatalf("callback = %d: %s", status, body)
	}
	stock, reserved := variantStock(t, env, order.variantID)

	overdue(t, env, order.number)
	expire(t, env)

	if got := orderStatusOf(t, env, order.number); got != "paid" {
		t.Errorf("status = %q, want paid", got)
	}
	stockAfter, reservedAfter := variantStock(t, env, order.variantID)
	if stockAfter != stock || reservedAfter != reserved {
		t.Errorf("the worker moved a paid order's stock: %d/%d, want %d/%d",
			stockAfter, reservedAfter, stock, reserved)
	}
	assertStockInvariant(t, env)
}

// The units held by several overdue orders come back in one pass, and the
// invariant holds over every size in the shop.
func TestExpiryWorkerHandlesABatch(t *testing.T) {
	env := startShopEnv(t)

	var numbers []string
	for _, qty := range []string{"1", "3", "2"} {
		order := checkout(t, env, qty)
		numbers = append(numbers, order.number)
		overdue(t, env, order.number)
	}

	expire(t, env)

	for _, number := range numbers {
		if got := orderStatusOf(t, env, number); got != "expired" {
			t.Errorf("order %s = %q, want expired", number, got)
		}
	}
	var stillReserved int
	err := env.store.Pool().QueryRow(context.Background(),
		`SELECT coalesce(sum(reserved), 0) FROM product_variants`).Scan(&stillReserved)
	if err != nil {
		t.Fatalf("sum reservations: %v", err)
	}
	if stillReserved != 0 {
		t.Errorf("reserved units left after the pass: %d", stillReserved)
	}
	assertStockInvariant(t, env)
}

// assertStockInvariant is the rule of tech.md §4: whatever happened, no size is
// oversold.
func assertStockInvariant(t *testing.T, env *shopEnv) {
	t.Helper()
	var broken int
	err := env.store.Pool().QueryRow(context.Background(),
		`SELECT count(*) FROM product_variants WHERE stock - reserved < 0 OR reserved < 0`).Scan(&broken)
	if err != nil {
		t.Fatalf("check stock invariant: %v", err)
	}
	if broken != 0 {
		t.Errorf("%d variants break stock - reserved >= 0", broken)
	}
}
