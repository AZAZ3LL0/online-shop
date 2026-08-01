package payment

import "time"

// LogEntry is one line of the raw provider log of an order, as the admin card
// lists it (tech.md §8.4). The decimals stay text: money never becomes a float.
type LogEntry struct {
	ProviderStatus    string
	ProviderPaymentID string
	SignatureOK       bool
	PayCurrency       string
	PayAmount         string
	ActuallyPaid      string
	ReceivedAt        time.Time
}

// IsPartial reports whether the newest event left the order short-paid. A
// partial payment never becomes paid on its own (tech.md §5.4), so the panel has
// to say so rather than leave an operator reading raw provider states.
// The log arrives newest first.
func IsPartial(log []LogEntry) bool {
	return len(log) > 0 && log[0].ProviderStatus == ProviderPartiallyPaid
}
