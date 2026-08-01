package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/qzq-kiim/shop/internal/domain/analytics"
	"github.com/qzq-kiim/shop/internal/storage/postgres/sqlcgen"
)

// AnalyticsRepo writes visits and events.
type AnalyticsRepo struct {
	q *sqlcgen.Queries
}

// NewAnalyticsRepo returns the analytics repository bound to the store.
func NewAnalyticsRepo(s *Store) *AnalyticsRepo { return &AnalyticsRepo{q: s.q} }

var _ analytics.Repository = (*AnalyticsRepo)(nil)

// RecordVisit stores one session landing.
func (r *AnalyticsRepo) RecordVisit(ctx context.Context, v analytics.Visit) error {
	err := r.q.InsertVisit(ctx, sqlcgen.InsertVisitParams{
		VisitorID:   v.Touch.VisitorID,
		SessionID:   v.SessionID,
		LandingPath: v.Touch.LandingPath,
		Referrer:    nullString(v.Touch.Referrer),
		UtmSource:   nullString(v.Touch.UTMSource),
		UtmMedium:   nullString(v.Touch.UTMMedium),
		UtmCampaign: nullString(v.Touch.UTMCampaign),
		UtmContent:  nullString(v.Touch.UTMContent),
		UtmTerm:     nullString(v.Touch.UTMTerm),
		UserAgent:   nullString(v.UserAgent),
		IsBot:       v.IsBot,
		CreatedAt:   ts(time.Now().UTC()),
	})
	if err != nil {
		return fmt.Errorf("insert visit: %w", err)
	}
	return nil
}

// RecordOrderPaid closes the funnel for one order. The provider callback carries
// no cookies, so the event is filed against the last session of the buyer.
func (r *AnalyticsRepo) RecordOrderPaid(ctx context.Context, orderID uuid.UUID) error {
	err := r.q.InsertOrderPaidEvent(ctx, sqlcgen.InsertOrderPaidEventParams{
		OrderID: orderID,
		Payload: []byte(`{}`),
	})
	if err != nil {
		return fmt.Errorf("insert order paid event: %w", err)
	}
	return nil
}

// RecordEvent appends one server-side event.
func (r *AnalyticsRepo) RecordEvent(ctx context.Context, visitorID, sessionID uuid.UUID, t analytics.EventType, payload []byte) error {
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	err := r.q.InsertEvent(ctx, sqlcgen.InsertEventParams{
		VisitorID: visitorID,
		SessionID: sessionID,
		Type:      string(t),
		Payload:   payload,
		CreatedAt: ts(time.Now().UTC()),
	})
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	return nil
}
