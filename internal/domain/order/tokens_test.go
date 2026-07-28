package order_test

import (
	"regexp"
	"testing"
	"time"

	"github.com/qzq-kiim/shop/internal/domain/order"
)

// draws is the sample size the S4.1 acceptance criteria names.
const draws = 10_000

var (
	reLinkCode    = regexp.MustCompile(`^[0-9a-f]{16}$`)
	rePublicToken = regexp.MustCompile(`^[0-9a-f]{32}$`)
	reNumber      = regexp.MustCompile(`^ORD-\d{6}-[0-9A-HJ-NP-Z]{4}$`)
)

// The deep link code addresses an order in the bot, so a repeat would hand one
// buyer's order to another. Ten thousand draws must all differ and all be the
// 16 hex chars of tech.md §4 (TASKS.md S4.1).
func TestLinkCodeIsUniqueAndWellFormed(t *testing.T) {
	seen := make(map[string]struct{}, draws)
	for i := range draws {
		code, err := order.NewLinkCode()
		if err != nil {
			t.Fatalf("draw %d: %v", i, err)
		}
		if !reLinkCode.MatchString(code) {
			t.Fatalf("draw %d = %q, want 16 hex chars", i, code)
		}
		if _, dup := seen[code]; dup {
			t.Fatalf("draw %d repeated an earlier code: %q", i, code)
		}
		seen[code] = struct{}{}
	}
}

// The status page token guards the only address of an order, so it carries the
// same uniqueness duty at twice the width.
func TestPublicTokenIsUniqueAndWellFormed(t *testing.T) {
	seen := make(map[string]struct{}, draws)
	for i := range draws {
		token, err := order.NewPublicToken()
		if err != nil {
			t.Fatalf("draw %d: %v", i, err)
		}
		if !rePublicToken.MatchString(token) {
			t.Fatalf("draw %d = %q, want 32 hex chars", i, token)
		}
		if _, dup := seen[token]; dup {
			t.Fatalf("draw %d repeated an earlier token: %q", i, token)
		}
		seen[token] = struct{}{}
	}
}

// The number is read back over the phone, so it stays inside the alphabet that
// has no look-alike characters and keeps the ORD-YYMMDD-XXXX shape of tech.md §4.
func TestNumberShape(t *testing.T) {
	now := time.Date(2026, 7, 28, 9, 0, 0, 0, time.UTC)
	for range 1_000 {
		number, err := order.NewNumber(now)
		if err != nil {
			t.Fatalf("new number: %v", err)
		}
		if !reNumber.MatchString(number) {
			t.Fatalf("number = %q, want ORD-260728-XXXX over the safe alphabet", number)
		}
		if got := number[4:10]; got != "260728" {
			t.Fatalf("number carries the date %q, want 260728", got)
		}
	}
}
