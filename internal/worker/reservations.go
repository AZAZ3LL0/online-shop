package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/qzq-kiim/shop/internal/domain/order"
)

// expiryBatch bounds one pass, so a backlog is drained over several ticks
// instead of holding one long transaction.
const expiryBatch = 100

// Expirer releases the stock held by orders that were never paid for. It is the
// only thing that ends a reservation on time; everything else waits for a
// provider callback that may never come (tech.md §4).
type Expirer struct {
	orders   order.Repository
	log      *slog.Logger
	interval time.Duration
	now      func() time.Time
}

// NewExpirer wires the reservation worker.
func NewExpirer(orders order.Repository, log *slog.Logger, interval time.Duration) *Expirer {
	return &Expirer{orders: orders, log: log, interval: interval, now: time.Now}
}

// Run releases expired reservations until the context is cancelled.
func (e *Expirer) Run(ctx context.Context) {
	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()

	for {
		if err := e.Tick(ctx); err != nil && !errors.Is(err, context.Canceled) {
			e.log.Error("reservation expiry pass failed", slog.String("error", err.Error()))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Tick expires one batch of due orders. A second pass over the same rows is a
// no-op: only orders still awaiting payment are selected, and an expired order
// is no longer one of them.
func (e *Expirer) Tick(ctx context.Context) error {
	expired, err := e.orders.ExpireDue(ctx, e.now().UTC(), expiryBatch)
	if err != nil {
		return err
	}
	if expired > 0 {
		e.log.Info("reservations released", slog.Int("orders", expired))
	}
	return nil
}
