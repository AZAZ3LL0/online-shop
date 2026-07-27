// Package analytics owns visitor attribution and the event stream the admin
// dashboard aggregates over.
package analytics

import (
	"context"

	"github.com/google/uuid"
)

// EventType enumerates the server-side events written where they happen.
type EventType string

// Event types, tech.md §4 (events.type).
const (
	EventPageView        EventType = "page_view"
	EventProductView     EventType = "product_view"
	EventAddToCart       EventType = "add_to_cart"
	EventRemoveFromCart  EventType = "remove_from_cart"
	EventCheckoutStarted EventType = "checkout_started"
	EventPaymentCreated  EventType = "payment_created"
	EventOrderPaid       EventType = "order_paid"
)

// Touch is one attribution snapshot, stored as jsonb in orders.first_touch and
// orders.last_touch and in the ft cookie.
type Touch struct {
	VisitorID   uuid.UUID `json:"visitor_id"`
	UTMSource   string    `json:"utm_source,omitempty"`
	UTMMedium   string    `json:"utm_medium,omitempty"`
	UTMCampaign string    `json:"utm_campaign,omitempty"`
	UTMContent  string    `json:"utm_content,omitempty"`
	UTMTerm     string    `json:"utm_term,omitempty"`
	Referrer    string    `json:"referrer,omitempty"`
	LandingPath string    `json:"landing_path,omitempty"`
}

// Visit is one session landing, written once per new sid.
type Visit struct {
	ID        uuid.UUID
	SessionID uuid.UUID
	Touch     Touch
	UserAgent string
	IsBot     bool
}

// Repository is the write access the attribution middleware needs.
type Repository interface {
	RecordVisit(ctx context.Context, v Visit) error
	RecordEvent(ctx context.Context, visitorID, sessionID uuid.UUID, t EventType, payload []byte) error
}
