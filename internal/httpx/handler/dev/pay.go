package dev

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/a-h/templ"

	"github.com/qzq-kiim/shop/internal/domain/order"
	"github.com/qzq-kiim/shop/internal/httpx/handler/webhook"
	"github.com/qzq-kiim/shop/internal/httpx/reqctx"
	"github.com/qzq-kiim/shop/internal/payments/nowpayments"
	"github.com/qzq-kiim/shop/web/templates"
	"github.com/qzq-kiim/shop/web/templates/pages"
)

// callbackPath is where the simulated callback is delivered, tech.md §5.4.
const callbackPath = "/webhooks/nowpayments"

// Payments is the local stand-in for the hosted NOWPayments invoice page. It
// does not fake the callback path: it builds a real body, signs it with the
// fake provider's secret and posts it through the real IPN endpoint, so the
// signature check and the status machine are exercised in development.
type Payments struct {
	fake        *nowpayments.Fake
	orders      *order.Service
	ipn         http.Handler
	botUsername string
	log         *slog.Logger
}

// NewPayments wires the development payment page. botUsername may be empty, in
// which case the page simply offers no Telegram entry point.
func NewPayments(fake *nowpayments.Fake, orders *order.Service, ipn http.Handler, botUsername string, log *slog.Logger) *Payments {
	return &Payments{fake: fake, orders: orders, ipn: ipn, botUsername: botUsername, log: log}
}

// Page renders the buttons that stand in for the provider's payment states.
func (h *Payments) Page(w http.ResponseWriter, r *http.Request) {
	found, ok := h.order(w, r)
	if !ok {
		return
	}
	page := templates.Page{
		Title:     "Dev payment " + found.Number,
		CSRFToken: reqctx.CSRFToken(r.Context()),
		IsDev:     true,
	}
	view := pages.DevPayView{
		Number:    found.Number,
		Total:     found.Total,
		OrderURL:  "/order/" + found.PublicToken,
		CSRFToken: page.CSRFToken,
		Statuses:  nowpayments.DevStatuses,
		Track:     pages.TrackEntry{BotUsername: h.botUsername, LinkCode: found.TGLinkCode},
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	ctx := templ.WithChildren(r.Context(), pages.DevPay(view))
	if err := templates.Shop(page).Render(ctx, w); err != nil {
		h.log.Error("render dev payment page failed",
			slog.String("request_id", reqctx.RequestID(r.Context())),
			slog.String("error", err.Error()))
	}
}

// Simulate delivers one signed callback with the chosen provider status and
// sends the buyer back to the order page.
func (h *Payments) Simulate(w http.ResponseWriter, r *http.Request) {
	found, ok := h.order(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	status := r.PostForm.Get("status")
	if !nowpayments.IsDevStatus(status) {
		http.Error(w, "unknown status", http.StatusBadRequest)
		return
	}

	body, err := json.Marshal(nowpayments.DevCallback(found.Number, status, found.Total))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	signature, err := h.fake.SignBody(body)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, callbackPath, bytes.NewReader(body))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(webhook.HeaderSignature, signature)

	rec := &recorder{code: http.StatusOK}
	h.ipn.ServeHTTP(rec, req)
	if rec.code != http.StatusOK {
		h.fail(w, r, fmt.Errorf("callback answered %d: %s", rec.code, rec.body.String()))
		return
	}
	http.Redirect(w, r, "/order/"+found.PublicToken, http.StatusSeeOther)
}

func (h *Payments) order(w http.ResponseWriter, r *http.Request) (order.Order, bool) {
	found, err := h.orders.ByNumber(r.Context(), r.PathValue("number"))
	if err != nil {
		if errors.Is(err, order.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return order.Order{}, false
		}
		h.fail(w, r, err)
		return order.Order{}, false
	}
	return found, true
}

func (h *Payments) fail(w http.ResponseWriter, r *http.Request, err error) {
	h.log.Error("dev payment failed",
		slog.String("request_id", reqctx.RequestID(r.Context())),
		slog.String("error", err.Error()))
	http.Error(w, "internal server error", http.StatusInternalServerError)
}

// recorder captures the answer of the in-process callback so the page can
// report a rejected simulation instead of silently redirecting.
type recorder struct {
	header http.Header
	body   bytes.Buffer
	code   int
}

func (r *recorder) Header() http.Header {
	if r.header == nil {
		r.header = make(http.Header)
	}
	return r.header
}

func (r *recorder) Write(b []byte) (int, error) { return r.body.Write(b) }

func (r *recorder) WriteHeader(code int) { r.code = code }
