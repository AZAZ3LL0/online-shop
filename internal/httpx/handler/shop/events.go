package shop

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/qzq-kiim/shop/internal/domain/analytics"
	"github.com/qzq-kiim/shop/internal/httpx/reqctx"
)

// recordPageView notes that a visitor opened one storefront page. Only pages go
// through here: the status poll of /order/{token}/status runs every ten seconds
// and would drown the funnel in noise.
func (h *Handler) recordPageView(r *http.Request) {
	h.recordEvent(r, analytics.EventPageView, payload(map[string]any{"path": r.URL.Path}))
}

// recordProductView notes which model was opened.
func (h *Handler) recordProductView(r *http.Request, productID uuid.UUID, slug string) {
	h.recordEvent(r, analytics.EventProductView, payload(map[string]any{
		"product_id": productID.String(),
		"slug":       slug,
	}))
}

// recordEvent writes one funnel event where it happens, tech.md §5.6. A failure
// to record must never break the purchase. Requests attribution classified as a
// crawler are dropped here, so bot traffic never reaches the funnel (§5.6).
func (h *Handler) recordEvent(r *http.Request, t analytics.EventType, body []byte) {
	if reqctx.IsBot(r.Context()) {
		return
	}
	touch, ok := reqctx.Touch(r.Context())
	if !ok {
		return
	}
	sessionID, ok := reqctx.SessionID(r.Context())
	if !ok {
		return
	}
	if err := h.analytics.RecordEvent(r.Context(), touch.VisitorID, sessionID, t, body); err != nil {
		h.log.Error("record event failed",
			slog.String("request_id", reqctx.RequestID(r.Context())),
			slog.String("event", string(t)),
			slog.String("error", err.Error()))
	}
}

// payload encodes an event payload, falling back to an empty object: an event
// worth counting is never dropped because its details did not serialise.
func payload(fields map[string]any) []byte {
	encoded, err := json.Marshal(fields)
	if err != nil {
		return nil
	}
	return encoded
}
