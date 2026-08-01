package order_test

import (
	"math/rand"
	"testing"

	"github.com/qzq-kiim/shop/internal/domain/order"
)

var all = []order.Status{
	order.StatusCreated,
	order.StatusAwaitingPayment,
	order.StatusPaid,
	order.StatusShipped,
	order.StatusDelivered,
	order.StatusExpired,
	order.StatusCancelled,
	order.StatusRefunded,
}

func TestCanTransitionMatchesDiagram(t *testing.T) {
	allowed := map[order.Status][]order.Status{
		order.StatusCreated:         {order.StatusAwaitingPayment},
		order.StatusAwaitingPayment: {order.StatusPaid, order.StatusExpired, order.StatusCancelled},
		order.StatusPaid:            {order.StatusShipped, order.StatusRefunded, order.StatusCancelled},
		order.StatusShipped:         {order.StatusDelivered},
		order.StatusExpired:         {order.StatusCancelled},
	}
	for _, from := range all {
		for _, to := range all {
			want := false
			for _, a := range allowed[from] {
				if a == to {
					want = true
				}
			}
			if got := order.CanTransition(from, to); got != want {
				t.Errorf("CanTransition(%s, %s) = %v, want %v", from, to, got, want)
			}
		}
	}
}

func TestNoSelfTransition(t *testing.T) {
	for _, s := range all {
		if order.CanTransition(s, s) {
			t.Errorf("status %s must not transition to itself", s)
		}
	}
}

// Applying any random permutation of status events must never walk backwards.
func TestNoRollbackUnderAnyEventOrder(t *testing.T) {
	rank := map[order.Status]int{
		order.StatusCreated:         0,
		order.StatusAwaitingPayment: 1,
		order.StatusPaid:            2,
		order.StatusShipped:         3,
		order.StatusDelivered:       4,
	}
	rng := rand.New(rand.NewSource(7))
	for range 500 {
		current := order.StatusCreated
		for range 20 {
			next := all[rng.Intn(len(all))]
			if !order.CanTransition(current, next) {
				continue
			}
			curRank, curOK := rank[current]
			nextRank, nextOK := rank[next]
			if curOK && nextOK && nextRank <= curRank {
				t.Fatalf("rollback %s -> %s", current, next)
			}
			current = next
		}
	}
}
