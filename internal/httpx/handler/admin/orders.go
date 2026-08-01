package admin

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/qzq-kiim/shop/internal/domain/order"
	"github.com/qzq-kiim/shop/internal/domain/payment"
	"github.com/qzq-kiim/shop/internal/httpx/reqctx"
	"github.com/qzq-kiim/shop/internal/telegram"
	"github.com/qzq-kiim/shop/web/templates"
	"github.com/qzq-kiim/shop/web/templates/pages"
)

// dateLayout is the format the period filter arrives in from an <input type=date>.
const dateLayout = "2006-01-02"

// maxNumberSearchRunes caps the order number search, counted in runes.
const maxNumberSearchRunes = 40

// PaymentLog is the provider log of one order, declared here by its consumer.
type PaymentLog interface {
	PaymentEventsByOrder(ctx context.Context, orderID uuid.UUID) ([]payment.LogEntry, error)
}

// ChatLinks are the Telegram chats following one order.
type ChatLinks interface {
	LinksByOrder(ctx context.Context, orderID uuid.UUID) ([]telegram.ChatLink, error)
}

// Orders renders one filtered page of the order list.
func (h *Handler) Orders(w http.ResponseWriter, r *http.Request) {
	form := orderFilterForm(r)
	filter, err := parseOrderFilter(form, r.URL.Query().Get("page"))
	if err != nil {
		// A filter the panel cannot parse is not an error page: the list comes
		// back unfiltered with the inputs the operator typed still in place.
		h.log.Warn("admin order filter ignored",
			slog.String("request_id", reqctx.RequestID(r.Context())),
			slog.String("error", err.Error()))
	}

	list, err := h.orders.List(r.Context(), filter)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	products, err := h.catalog.ListActive(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}

	h.renderPage(w, r, "Orders", templates.SectionOrders, pages.AdminOrders(pages.AdminOrdersView{
		List:     list,
		Filter:   form,
		Products: products,
	}))
}

// Order renders one order card.
func (h *Handler) Order(w http.ResponseWriter, r *http.Request) {
	id, ok := h.orderID(w, r)
	if !ok {
		return
	}
	view, err := h.orderView(r, id, "")
	if err != nil {
		h.orderFailure(w, r, err)
		return
	}
	h.renderPage(w, r, "Order "+view.Detail.Order.Number, templates.SectionOrders, pages.AdminOrder(view))
}

// OrderStatus applies a manual transition and shows the card again. A move the
// status machine refuses answers 409 with the reason on the card.
func (h *Handler) OrderStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := h.orderID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	target := order.Status(strings.TrimSpace(r.PostForm.Get("status")))

	_, err := h.orders.ChangeStatus(r.Context(), id, target)
	switch {
	case err == nil:
		http.Redirect(w, r, "/admin/orders/"+id.String(), http.StatusSeeOther)
		return
	case errors.Is(err, order.ErrTransitionNotAllowed), errors.Is(err, order.ErrConflict):
		view, viewErr := h.orderView(r, id, "That move is not allowed from the current status.")
		if viewErr != nil {
			h.orderFailure(w, r, viewErr)
			return
		}
		h.renderPageStatus(w, r, http.StatusConflict,
			"Order "+view.Detail.Order.Number, templates.SectionOrders, pages.AdminOrder(view))
		return
	default:
		h.orderFailure(w, r, err)
	}
}

// orderView assembles one card out of the order, its provider log and the chats
// following it.
func (h *Handler) orderView(r *http.Request, id uuid.UUID, message string) (pages.AdminOrderView, error) {
	detail, err := h.orders.Detail(r.Context(), id)
	if err != nil {
		return pages.AdminOrderView{}, err
	}
	events, err := h.payments.PaymentEventsByOrder(r.Context(), id)
	if err != nil {
		return pages.AdminOrderView{}, err
	}
	chats, err := h.links.LinksByOrder(r.Context(), id)
	if err != nil {
		return pages.AdminOrderView{}, err
	}
	return pages.AdminOrderView{
		Detail:    detail,
		Payments:  events,
		Chats:     chats,
		CSRFToken: reqctx.CSRFToken(r.Context()),
		Error:     message,
	}, nil
}

// orderID reads the order id out of the path.
func (h *Handler) orderID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) orderFailure(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, order.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	h.fail(w, r, err)
}

// orderFilterForm reads the filter bar as the browser sent it.
func orderFilterForm(r *http.Request) pages.OrderFilterForm {
	q := r.URL.Query()
	return pages.OrderFilterForm{
		Status:    strings.TrimSpace(q.Get("status")),
		From:      strings.TrimSpace(q.Get("from")),
		To:        strings.TrimSpace(q.Get("to")),
		ProductID: strings.TrimSpace(q.Get("product")),
		Number:    strings.TrimSpace(q.Get("number")),
	}
}

// parseOrderFilter turns the submitted bar into a domain filter. Every input is
// validated here; an unusable value is dropped and reported, never passed on.
func parseOrderFilter(form pages.OrderFilterForm, page string) (order.Filter, error) {
	var problems []string
	filter := order.Filter{Page: 1, PageSize: order.DefaultPageSize}

	if n, err := strconv.Atoi(page); err == nil && n > 1 {
		filter.Page = n
	}
	if form.Status != "" {
		if status := order.Status(form.Status); order.Valid(status) {
			filter.Status = status
		} else {
			problems = append(problems, "unknown status "+form.Status)
		}
	}
	if from, ok := parseDate(form.From, &problems, "from"); ok {
		filter.From = &from
	}
	if to, ok := parseDate(form.To, &problems, "to"); ok {
		// The bound is exclusive, so a day picked as "to" is included whole.
		end := to.AddDate(0, 0, 1)
		filter.To = &end
	}
	if form.ProductID != "" {
		if id, err := uuid.Parse(form.ProductID); err == nil {
			filter.ProductID = &id
		} else {
			problems = append(problems, "unknown product filter")
		}
	}
	if number := form.Number; number != "" {
		if len([]rune(number)) > maxNumberSearchRunes {
			problems = append(problems, "the number search is too long")
		} else {
			filter.Number = number
		}
	}

	if len(problems) > 0 {
		return filter, errors.New(strings.Join(problems, "; "))
	}
	return filter, nil
}

func parseDate(value string, problems *[]string, field string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(dateLayout, value)
	if err != nil {
		*problems = append(*problems, "unreadable "+field+" date")
		return time.Time{}, false
	}
	return parsed.UTC(), true
}
