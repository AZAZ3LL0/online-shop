// Package pages holds the page-level templates. Every visual primitive they use
// comes from the components package.
package pages

import (
	"encoding/json"
	"time"

	"github.com/a-h/templ"

	"github.com/qzq-kiim/shop/internal/domain/cart"
	"github.com/qzq-kiim/shop/internal/domain/catalog"
	"github.com/qzq-kiim/shop/internal/domain/order"
	"github.com/qzq-kiim/shop/internal/money"
	"github.com/qzq-kiim/shop/internal/telegram"
	"github.com/qzq-kiim/shop/web/templates/components"
)

// lowStockThreshold is where a size starts being flagged as running out. The
// exact stock count is never published to visitors.
const lowStockThreshold = 3

// CartView is everything the cart fragment needs to render itself.
type CartView struct {
	Cart      cart.Cart
	Subtotal  money.Amount
	Shipping  money.Amount
	Total     money.Amount
	CSRFToken string
	Error     string
}

// HomeView is the storefront index.
type HomeView struct {
	Products  []catalog.Product
	Cart      CartView
	CSRFToken string
}

// ProductView is one product page: the model, the cart panel and the token the
// add form has to carry.
type ProductView struct {
	Product   catalog.Product
	Cart      CartView
	CSRFToken string
}

// CheckoutView is the checkout form: the priced cart, what the buyer typed and
// what still has to be fixed.
type CheckoutView struct {
	Cart               CartView
	Customer           order.Customer
	Consent            bool
	Errors             order.FieldErrors
	Error              string
	CSRFToken          string
	ReservationMinutes int
	// Paused mirrors settings.shop_paused: the form is shown, but the order
	// cannot be placed.
	Paused bool
}

// FieldError returns the message shown under one field, empty when it is fine.
func (v CheckoutView) FieldError(name string) string { return v.Errors[name] }

// TrackEntry is the "Track order in Telegram" block. Both pages that offer the
// bot embed it, so the link and the QR are built in one place (tech.md §5.5).
type TrackEntry struct {
	BotUsername string
	LinkCode    string
}

// Offered reports whether a bot entry point can be shown at all.
func (t TrackEntry) Offered() bool { return t.URL() != "" }

// URL is the bot deep link of this order, empty when no bot is configured.
func (t TrackEntry) URL() string { return telegram.DeepLink(t.BotUsername, t.LinkCode) }

// QR is the deep link as an inline PNG, for scanning from a desktop screen. A
// code that cannot be drawn costs the button nothing, so the error is dropped
// here rather than failing the whole page.
func (t TrackEntry) QR() string {
	uri, err := telegram.QRDataURI(t.URL())
	if err != nil {
		return ""
	}
	return uri
}

// OrderView is the public status page of one order.
type OrderView struct {
	Order order.Order
	Now   time.Time
	Track TrackEntry
}

// Expired reports whether the reservation deadline has passed.
func (v OrderView) Expired() bool { return v.Order.IsExpired(v.Now) }

// Payable reports whether the buyer can still be sent to the invoice.
func (v OrderView) Payable() bool {
	return v.Order.InvoiceURL != "" && v.Order.Status == order.StatusAwaitingPayment && !v.Expired()
}

// orderState builds the Alpine state of the status page: where to poll and when
// the reservation runs out. The status itself stays server-rendered; the page
// reloads when the poll reports a different one.
func orderState(v OrderView) string {
	state := map[string]any{
		"status":    string(v.Order.Status),
		"statusUrl": "/order/" + v.Order.PublicToken + "/status",
		"expiresAt": "",
	}
	if v.Order.ExpiresAt != nil {
		state["expiresAt"] = v.Order.ExpiresAt.UTC().Format(time.RFC3339)
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		return "orderStatus({})"
	}
	return "orderStatus(" + string(encoded) + ")"
}

// DevPayView is the local stand-in for the hosted invoice page, rendered only
// in a development build.
type DevPayView struct {
	Number    string
	Total     money.Amount
	OrderURL  string
	CSRFToken string
	Statuses  []string
	Track     TrackEntry
}

// TelegramLaunchView is the Mini App entry page: where to post the launch
// payload and what went wrong if the last attempt was refused.
type TelegramLaunchView struct {
	AuthURL   string
	CSRFToken string
	Error     string
}

// LoginView is the admin login form state.
type LoginView struct {
	CSRFToken string
	Login     string
	Error     string
}

// sizeOptions turns variants into Select options, marking sold out sizes.
func sizeOptions(p catalog.Product) []components.Option {
	options := make([]components.Option, 0, len(p.Variants))
	for _, v := range p.Variants {
		opt := components.Option{Value: v.ID.String(), Label: v.Size}
		if v.Available() <= 0 {
			opt.Disabled = true
			opt.Note = "sold out"
		}
		options = append(options, opt)
	}
	return options
}

// availability is the count-free description of one size shown to visitors.
type availability struct {
	Label string
	Tone  components.Tone
}

// sizeAvailability grades one size. Availability is always derived from
// Variant.Available(), never from Stock alone: reserved units are already sold
// as far as the storefront is concerned.
func sizeAvailability(v catalog.Variant) availability {
	switch left := v.Available(); {
	case left <= 0:
		return availability{Label: "sold out", Tone: components.ToneDanger}
	case left <= lowStockThreshold:
		return availability{Label: "low stock", Tone: components.ToneWarning}
	default:
		return availability{Label: "in stock", Tone: components.ToneSuccess}
	}
}

// sizeRows renders the size run of a product as table rows.
func sizeRows(p catalog.Product) []templ.Component {
	rows := make([]templ.Component, 0, len(p.Variants))
	for _, v := range p.Variants {
		rows = append(rows, sizeRow(v.Size, sizeAvailability(v)))
	}
	return rows
}

// inStock reports whether any size of the product can still be bought.
func inStock(p catalog.Product) bool {
	for _, v := range p.Variants {
		if v.Available() > 0 {
			return true
		}
	}
	return false
}
