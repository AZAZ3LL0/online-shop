package shop

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/qzq-kiim/shop/internal/domain/order"
	"github.com/qzq-kiim/shop/internal/httpx/reqctx"
	"github.com/qzq-kiim/shop/web/templates"
	"github.com/qzq-kiim/shop/web/templates/pages"
)

// statusResponse is the JSON the status page polls every ten seconds. It says
// nothing beyond what the page already shows.
type statusResponse struct {
	Status    string `json:"status"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

// OrderPage renders the public status page of one order.
func (h *Handler) OrderPage(w http.ResponseWriter, r *http.Request) {
	found, ok := h.orderByToken(w, r)
	if !ok {
		return
	}
	page := templates.Page{
		Title:     "Order " + found.Number + " - QZQ",
		CSRFToken: reqctx.CSRFToken(r.Context()),
		IsDev:     h.isDev,
	}
	h.recordPageView(r)
	view := pages.OrderView{Order: found, Now: time.Now().UTC()}
	h.render(w, r, templates.Shop(page).Render, pages.OrderStatus(view))
}

// OrderStatus answers the poll of the status page.
func (h *Handler) OrderStatus(w http.ResponseWriter, r *http.Request) {
	found, ok := h.orderByToken(w, r)
	if !ok {
		return
	}
	body := statusResponse{Status: string(found.Status)}
	if found.ExpiresAt != nil {
		body.ExpiresAt = found.ExpiresAt.UTC().Format(time.RFC3339)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(body); err != nil {
		h.log.Error("encode order status failed",
			slog.String("request_id", reqctx.RequestID(r.Context())),
			slog.String("error", err.Error()))
	}
}

// PaymentReturn is where the provider sends the buyer back to. The status comes
// from the IPN, never from this redirect, so it only leads back to the page.
func (h *Handler) PaymentReturn(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/order/"+r.PathValue("token"), http.StatusSeeOther)
}

// orderByToken resolves the public token. An unknown token gets the same
// neutral 404 as any other missing page: whether an order exists is not
// something the shop confirms to a guesser.
func (h *Handler) orderByToken(w http.ResponseWriter, r *http.Request) (order.Order, bool) {
	found, err := h.orders.ByPublicToken(r.Context(), r.PathValue("token"))
	if err != nil {
		if errors.Is(err, order.ErrNotFound) {
			h.notFound(w, r)
			return order.Order{}, false
		}
		h.fail(w, r, err)
		return order.Order{}, false
	}
	return found, true
}
