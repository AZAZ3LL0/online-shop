package shop

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/a-h/templ"
	"github.com/google/uuid"

	"github.com/qzq-kiim/shop/internal/domain/cart"
	"github.com/qzq-kiim/shop/internal/domain/catalog"
	"github.com/qzq-kiim/shop/internal/httpx/reqctx"
	"github.com/qzq-kiim/shop/web/templates"
	"github.com/qzq-kiim/shop/web/templates/pages"
)

// cartID reads the cart the browser carries in its signed cookie. A missing or
// tampered cookie is simply "no cart yet", never an error: a stale cookie must
// not break the storefront.
func (h *Handler) cartID(r *http.Request) *uuid.UUID {
	raw, ok := h.cookies.Get(r, CookieCart)
	if !ok {
		return nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil
	}
	return &id
}

// Messages the cart fragment shows on a rejected mutation. They are defined
// once so a bad quantity reads the same whether the form sent a word or an
// out-of-range number, and they never mention anything internal.
const (
	badQtyMessage      = "Quantity must be between 1 and 10."
	missingItemMessage = "That item is no longer in your cart."
	outOfStockMessage  = "Not enough stock left for that size."
)

// ensureCart resolves the cart from the signed cookie, creating one when the
// cookie is missing or points at a cart that no longer exists. Only mutations
// call it: plain browsing must not open a cart row per visitor.
func (h *Handler) ensureCart(w http.ResponseWriter, r *http.Request) (uuid.UUID, error) {
	current := h.cartID(r)
	var visitorID *uuid.UUID
	if touch, ok := reqctx.Touch(r.Context()); ok {
		visitorID = &touch.VisitorID
	}

	cartID, err := h.carts.EnsureCart(r.Context(), current, visitorID)
	if err != nil {
		return uuid.Nil, err
	}
	if current == nil || *current != cartID {
		h.cookies.Set(w, CookieCart, cartID.String(), cartTTL)
	}
	return cartID, nil
}

func (h *Handler) cartView(r *http.Request, cartID *uuid.UUID) (pages.CartView, error) {
	c, totals, err := h.carts.View(r.Context(), cartID)
	if err != nil {
		return pages.CartView{}, err
	}
	return pages.CartView{
		Cart:      c,
		Subtotal:  totals.Subtotal,
		Shipping:  totals.Shipping,
		Total:     totals.Total,
		CSRFToken: reqctx.CSRFToken(r.Context()),
	}, nil
}

// render writes a full 200 page: the layout wraps the body component.
func (h *Handler) render(w http.ResponseWriter, r *http.Request, layout func(context.Context, io.Writer) error, body templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.renderBody(w, r, layout, body)
}

// renderBody writes the layout with the body. The caller has already set the
// content type and, when it is not 200, the status code.
func (h *Handler) renderBody(w http.ResponseWriter, r *http.Request, layout func(context.Context, io.Writer) error, body templ.Component) {
	ctx := templ.WithChildren(r.Context(), body)
	if err := layout(ctx, w); err != nil {
		h.log.Error("render page failed",
			slog.String("request_id", reqctx.RequestID(r.Context())),
			slog.String("error", err.Error()))
	}
}

// writeFragment answers a cart mutation with the cart fragment only.
func (h *Handler) writeFragment(w http.ResponseWriter, r *http.Request, cartID *uuid.UUID, status int, message string) {
	view, err := h.cartView(r, cartID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	view.Error = message

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := pages.CartFragment(view).Render(r.Context(), w); err != nil {
		h.log.Error("render cart fragment failed",
			slog.String("request_id", reqctx.RequestID(r.Context())),
			slog.String("error", err.Error()))
	}
}

func (h *Handler) fragmentError(w http.ResponseWriter, r *http.Request, cartID *uuid.UUID, status int, message string) {
	h.writeFragment(w, r, cartID, status, message)
}

// fragmentFailure maps a domain error onto a status code and a message that
// gives nothing internal away.
func (h *Handler) fragmentFailure(w http.ResponseWriter, r *http.Request, cartID *uuid.UUID, err error) {
	switch {
	case errors.Is(err, cart.ErrCartItemLimit):
		h.writeFragment(w, r, cartID, http.StatusUnprocessableEntity, badQtyMessage)
	case errors.Is(err, catalog.ErrOutOfStock):
		h.writeFragment(w, r, cartID, http.StatusConflict, outOfStockMessage)
	case errors.Is(err, cart.ErrItemNotFound), errors.Is(err, catalog.ErrNotFound):
		h.writeFragment(w, r, cartID, http.StatusNotFound, missingItemMessage)
	default:
		h.fail(w, r, err)
	}
}

// notFound answers with the shared error page. Every storefront 404 goes
// through here, so the wording is defined once and says the same thing whether
// the slug never existed or the model was taken off sale.
func (h *Handler) notFound(w http.ResponseWriter, r *http.Request) {
	h.errorPage(w, r, http.StatusNotFound, "Not found", "This page is not on sale, or it never was.")
}

// errorPage renders the storefront error body inside the normal layout.
func (h *Handler) errorPage(w http.ResponseWriter, r *http.Request, status int, title, hint string) {
	page := templates.Page{
		Title:     title + " - QZQ",
		CSRFToken: reqctx.CSRFToken(r.Context()),
		IsDev:     h.isDev,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)

	ctx := templ.WithChildren(r.Context(), pages.ErrorPage(title, hint))
	if err := templates.Shop(page).Render(ctx, w); err != nil {
		h.log.Error("render error page failed",
			slog.String("request_id", reqctx.RequestID(r.Context())),
			slog.String("error", err.Error()))
	}
}

// fail logs the cause and answers with a generic message.
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	h.log.Error("storefront request failed",
		slog.String("request_id", reqctx.RequestID(r.Context())),
		slog.String("path", r.URL.Path),
		slog.String("error", err.Error()))
	http.Error(w, "internal server error", http.StatusInternalServerError)
}
