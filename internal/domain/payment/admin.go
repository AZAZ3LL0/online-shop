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
