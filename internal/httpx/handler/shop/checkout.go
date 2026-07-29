package shop

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/qzq-kiim/shop/internal/domain/analytics"
	"github.com/qzq-kiim/shop/internal/domain/cart"
	"github.com/qzq-kiim/shop/internal/domain/catalog"
	"github.com/qzq-kiim/shop/internal/domain/order"
	"github.com/qzq-kiim/shop/internal/domain/payment"
	"github.com/qzq-kiim/shop/internal/httpx/middleware"
	"github.com/qzq-kiim/shop/internal/httpx/reqctx"
	"github.com/qzq-kiim/shop/internal/payments/nowpayments"
	"github.com/qzq-kiim/shop/web/templates"
	"github.com/qzq-kiim/shop/web/templates/pages"
)

// pausedMessage is what a buyer is told while the shop is paused. It says what
// is happening without blaming the buyer's cart, which is still intact.
const pausedMessage = "The shop is on pause right now, so no new order can be placed. Your cart is kept."

// CheckoutForm shows the buyer form. An empty cart has nothing to check out.
func (h *Handler) CheckoutForm(w http.ResponseWriter, r *http.Request) {
	view, ok := h.checkoutView(w, r)
	if !ok {
		return
	}
	if view.Paused {
		view.Error = pausedMessage
	}
	h.recordEvent(r, analytics.EventCheckoutStarted, nil)
	h.renderCheckout(w, r, view, http.StatusOK)
}

// Checkout validates the form, places the order with its price snapshot and
// stock reservation, creates the invoice and sends the buyer to it.
func (h *Handler) Checkout(w http.ResponseWriter, r *http.Request) {
	view, ok := h.checkoutView(w, r)
	if !ok {
		return
	}
	if view.Paused {
		// A paused shop reserves nothing and charges nothing (tech.md §5.3,
		// settings.shop_paused).
		view.Error = pausedMessage
		h.renderCheckout(w, r, view, http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		view.Error = "Malformed request."
		h.renderCheckout(w, r, view, http.StatusBadRequest)
		return
	}

	view.Customer = order.Customer{
		Name:    r.PostForm.Get("name"),
		Contact: r.PostForm.Get("contact"),
		Address: r.PostForm.Get("address"),
		Comment: r.PostForm.Get("comment"),
	}
	view.Consent = r.PostForm.Get("consent") != ""
	if !view.Consent {
		view.Errors = order.FieldErrors{"consent": "Tick the box to place the order."}
		h.renderCheckout(w, r, view, http.StatusUnprocessableEntity)
		return
	}

	placed, fieldErrors, err := h.orders.Place(r.Context(), h.placeInput(r, view))
	switch {
	case errors.Is(err, order.ErrValidation):
		view.Errors = fieldErrors
		if len(fieldErrors) == 0 {
			view.Error = "Your cart is empty."
		}
		h.renderCheckout(w, r, view, http.StatusUnprocessableEntity)
		return
	case errors.Is(err, catalog.ErrOutOfStock):
		view.Error = "One of the sizes ran out while you were typing. Check your cart."
		h.renderCheckout(w, r, view, http.StatusConflict)
		return
	case err != nil:
		h.fail(w, r, err)
		return
	}

	invoice, err := h.invoice(r, placed)
	if err != nil {
		// The buyer never got a payment page, so the units must not stay held.
		if releaseErr := h.orders.Abandon(r.Context(), placed.ID); releaseErr != nil {
			h.log.Error("release reservation failed",
				slog.String("request_id", reqctx.RequestID(r.Context())),
				slog.String("order", placed.Number),
				slog.String("error", releaseErr.Error()))
		}
		h.log.Error("create invoice failed",
			slog.String("request_id", reqctx.RequestID(r.Context())),
			slog.String("order", placed.Number),
			slog.String("error", err.Error()))
		view.Error = "The payment provider did not answer. Nothing was charged, try again in a minute."
		h.renderCheckout(w, r, view, http.StatusServiceUnavailable)
		return
	}

	if err := h.orders.AwaitPayment(r.Context(), placed, paymentRef(invoice)); err != nil {
		h.fail(w, r, err)
		return
	}
	h.recordEvent(r, analytics.EventPaymentCreated, nil)

	// The order owns the reservation now; a fresh cart keeps the buyer from
	// ordering the same tees twice by pressing back.
	h.cookies.Clear(w, CookieCart)
	http.Redirect(w, r, invoice.URL, http.StatusSeeOther)
}

// checkoutView loads the priced cart. It answers the request itself and returns
// false when there is nothing to check out.
func (h *Handler) checkoutView(w http.ResponseWriter, r *http.Request) (pages.CheckoutView, bool) {
	cartID, err := h.ensureCart(w, r)
	if err != nil {
		h.fail(w, r, err)
		return pages.CheckoutView{}, false
	}
	cartView, err := h.cartView(r, cartID)
	if err != nil {
		h.fail(w, r, err)
		return pages.CheckoutView{}, false
	}
	if cartView.Cart.IsEmpty() {
		http.Redirect(w, r, "/cart", http.StatusSeeOther)
		return pages.CheckoutView{}, false
	}

	shop, err := h.settings.Values(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return pages.CheckoutView{}, false
	}
	return pages.CheckoutView{
		Cart:               cartView,
		CSRFToken:          reqctx.CSRFToken(r.Context()),
		ReservationMinutes: shop.TTLMinutes(),
		Paused:             shop.ShopPaused,
	}, true
}

func (h *Handler) placeInput(r *http.Request, view pages.CheckoutView) order.PlaceInput {
	in := order.PlaceInput{
		Customer: view.Customer,
		Items:    orderItems(view.Cart.Cart),
		Subtotal: view.Cart.Subtotal,
		Shipping: view.Cart.Shipping,
		Total:    view.Cart.Total,
	}
	if touch, ok := reqctx.Touch(r.Context()); ok {
		visitorID := touch.VisitorID
		in.VisitorID = &visitorID
		in.LastTouch = touch
		in.FirstTouch = touch
	}
	if first, ok := middleware.FirstTouch(r, h.cookies); ok {
		in.FirstTouch = first
	}
	return in
}

// invoice asks the provider for a hosted payment page for this order.
func (h *Handler) invoice(r *http.Request, placed order.Order) (nowpayments.Invoice, error) {
	return h.payments.CreateInvoice(r.Context(), nowpayments.InvoiceRequest{
		Total:          placed.Total,
		OrderNumber:    placed.Number,
		Description:    "Merch order " + placed.Number,
		IPNCallbackURL: h.baseURL + "/webhooks/nowpayments",
		SuccessURL:     h.baseURL + "/payment/return/" + placed.PublicToken,
		CancelURL:      h.baseURL + "/order/" + placed.PublicToken,
	})
}

// orderItems snapshots the cart lines: the order keeps the price it was placed
// at, whatever the catalogue does afterwards.
func orderItems(c cart.Cart) []order.Item {
	items := make([]order.Item, 0, len(c.Items))
	for _, item := range c.Items {
		items = append(items, order.Item{
			VariantID:    item.VariantID,
			ProductTitle: item.ProductTitle,
			Size:         item.Size,
			UnitPrice:    item.UnitPrice,
			Qty:          item.Qty,
		})
	}
	return items
}

func paymentRef(invoice nowpayments.Invoice) order.PaymentRef {
	return order.PaymentRef{
		Provider:          payment.ProviderNOWPayments,
		ProviderPaymentID: invoice.ID,
		InvoiceURL:        invoice.URL,
		Status:            payment.ProviderWaiting,
	}
}

// recordEvent writes one funnel event where it happens, tech.md §5.6. A failure
// to record must never break the purchase.
func (h *Handler) recordEvent(r *http.Request, t analytics.EventType, payload []byte) {
	touch, ok := reqctx.Touch(r.Context())
	if !ok {
		return
	}
	sessionID, ok := reqctx.SessionID(r.Context())
	if !ok {
		return
	}
	if err := h.analytics.RecordEvent(r.Context(), touch.VisitorID, sessionID, t, payload); err != nil {
		h.log.Error("record event failed",
			slog.String("request_id", reqctx.RequestID(r.Context())),
			slog.String("event", string(t)),
			slog.String("error", err.Error()))
	}
}

func (h *Handler) renderCheckout(w http.ResponseWriter, r *http.Request, view pages.CheckoutView, status int) {
	page := templates.Page{
		Title:     "Checkout - QZQ",
		CartCount: view.Cart.Cart.Count(),
		CSRFToken: view.CSRFToken,
		IsDev:     h.isDev,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	h.renderBody(w, r, templates.Shop(page).Render, pages.Checkout(view))
}
