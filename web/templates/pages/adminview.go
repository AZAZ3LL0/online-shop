package pages

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/google/uuid"

	"github.com/qzq-kiim/shop/internal/domain/analytics"
	"github.com/qzq-kiim/shop/internal/domain/catalog"
	"github.com/qzq-kiim/shop/internal/domain/order"
	"github.com/qzq-kiim/shop/internal/domain/payment"
	"github.com/qzq-kiim/shop/internal/domain/settings"
	"github.com/qzq-kiim/shop/internal/money"
	"github.com/qzq-kiim/shop/internal/telegram"
	"github.com/qzq-kiim/shop/web/templates/components"
)

// adminTimeLayout is how the panel prints a timestamp. One layout for every
// admin page, so no template formats a time of its own.
const adminTimeLayout = "2006-01-02 15:04"

// OrderFilterForm is the order filter bar exactly as the browser sent it. The
// strings are echoed back into the form so a filtered page keeps its inputs; the
// parsed values live in order.Filter.
type OrderFilterForm struct {
	Status    string
	From      string
	To        string
	ProductID string
	Number    string
}

// Query renders the filter back into a query string so the pagination links and
// the row links keep the current view.
func (f OrderFilterForm) Query() url.Values {
	q := url.Values{}
	for field, value := range map[string]string{
		"status":  f.Status,
		"from":    f.From,
		"to":      f.To,
		"product": f.ProductID,
		"number":  f.Number,
	} {
		if value != "" {
			q.Set(field, value)
		}
	}
	return q
}

// AdminOrdersView is one page of the admin order list with its filter bar.
type AdminOrdersView struct {
	List     order.List
	Filter   OrderFilterForm
	Products []catalog.Product
}

// BaseURL is what Pagination appends the page number to: the same list under the
// same filter.
func (v AdminOrdersView) BaseURL() string {
	q := v.Filter.Query()
	if len(q) == 0 {
		return "/admin/orders"
	}
	return "/admin/orders?" + q.Encode()
}

// statusOptions are the status choices of the filter bar, in machine order.
func (v AdminOrdersView) statusOptions() []components.Option {
	statuses := []order.Status{
		order.StatusCreated, order.StatusAwaitingPayment, order.StatusPaid,
		order.StatusShipped, order.StatusDelivered, order.StatusExpired,
		order.StatusCancelled, order.StatusRefunded,
	}
	options := make([]components.Option, 0, len(statuses)+1)
	options = append(options, components.Option{Value: "", Label: "any status"})
	for _, s := range statuses {
		options = append(options, components.Option{Value: string(s), Label: string(s)})
	}
	return options
}

// productOptions are the product choices of the filter bar.
func (v AdminOrdersView) productOptions() []components.Option {
	options := make([]components.Option, 0, len(v.Products)+1)
	options = append(options, components.Option{Value: "", Label: "any model"})
	for _, p := range v.Products {
		options = append(options, components.Option{Value: p.ID.String(), Label: p.Title})
	}
	return options
}

// rows renders the list as table rows for the Table component.
func (v AdminOrdersView) rows() []templ.Component {
	rows := make([]templ.Component, 0, len(v.List.Orders))
	for _, o := range v.List.Orders {
		rows = append(rows, adminOrderRow(o))
	}
	return rows
}

// AdminOrderView is one order card: the order, the provider log behind it and
// the chats following it.
type AdminOrderView struct {
	Detail    order.Detail
	Payments  []payment.LogEntry
	Chats     []telegram.ChatLink
	CSRFToken string
	Error     string
}

// PartiallyPaid reports whether the newest provider event left the order short.
func (v AdminOrderView) PartiallyPaid() bool { return payment.IsPartial(v.Payments) }

// StatusFormAction is where a manual transition is posted.
func (v AdminOrderView) StatusFormAction() string {
	return "/admin/orders/" + v.Detail.Order.ID.String() + "/status"
}

// itemRows renders the order lines as table rows.
func (v AdminOrderView) itemRows() []templ.Component {
	rows := make([]templ.Component, 0, len(v.Detail.Order.Items))
	for _, item := range v.Detail.Order.Items {
		rows = append(rows, adminOrderItemRow(item))
	}
	return rows
}

// paymentRows renders the provider log as table rows.
func (v AdminOrderView) paymentRows() []templ.Component {
	rows := make([]templ.Component, 0, len(v.Payments))
	for _, entry := range v.Payments {
		rows = append(rows, adminPaymentRow(entry))
	}
	return rows
}

// adminTime prints a timestamp for the panel, empty when there is none.
func adminTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(adminTimeLayout)
}

// adminTimeOr prints an optional timestamp, with a dash when it is not set.
func adminTimeOr(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return adminTime(*t)
}

// signatureTone colours a provider log line by whether its signature verified.
func signatureTone(ok bool) components.Tone {
	if ok {
		return components.ToneSuccess
	}
	return components.ToneDanger
}

// touchRows renders one attribution snapshot as label/value pairs. Empty fields
// are dropped: an untagged visit should not print five empty lines.
func touchRows(label string, t analytics.Touch) []adminFact {
	facts := []adminFact{}
	for _, f := range []adminFact{
		{Label: label + " source", Value: t.UTMSource},
		{Label: label + " medium", Value: t.UTMMedium},
		{Label: label + " campaign", Value: t.UTMCampaign},
		{Label: label + " content", Value: t.UTMContent},
		{Label: label + " term", Value: t.UTMTerm},
		{Label: label + " referrer", Value: t.Referrer},
		{Label: label + " landing", Value: t.LandingPath},
	} {
		if f.Value != "" {
			facts = append(facts, f)
		}
	}
	if len(facts) == 0 {
		facts = append(facts, adminFact{Label: label, Value: "direct, untagged"})
	}
	return facts
}

// statusButtonVariant colours a manual transition by what it does: cancelling an
// order is the destructive one.
func statusButtonVariant(to order.Status) components.ButtonVariant {
	if to == order.StatusCancelled {
		return components.ButtonDanger
	}
	return components.ButtonPrimary
}

// signatureLabel is how a provider event reports its signature check.
func signatureLabel(ok bool) string {
	if ok {
		return "verified"
	}
	return "rejected"
}

// amountLabel prints a provider decimal with its currency, and a dash when the
// callback carried none.
func amountLabel(amount, currency string) string {
	if amount == "" {
		return "-"
	}
	if currency == "" {
		return amount
	}
	return amount + " " + strings.ToUpper(currency)
}

// AdminNotice is the message a rejected edit leaves on the product page, tied to
// the model it belongs to.
type AdminNotice struct {
	ProductID uuid.UUID
	Error     string
}

// On reports whether the notice belongs to this product.
func (n AdminNotice) On(productID uuid.UUID) bool {
	return n.Error != "" && n.ProductID == productID
}

// AdminProductsView is the product page: every model with its sizes and its
// price trail (tech.md §8.3).
type AdminProductsView struct {
	Products  []catalog.Product
	History   map[uuid.UUID][]catalog.PriceEntry
	Notice    AdminNotice
	Saved     string
	CSRFToken string
}

// SavedMessage is what the page says after a successful edit.
func (v AdminProductsView) SavedMessage() string {
	switch v.Saved {
	case "price":
		return "The new price is live, and the change is on the record."
	case "stock":
		return "The stock is updated."
	default:
		return ""
	}
}

// trail is the price history of one model, oldest changes last.
func (v AdminProductsView) trail(productID uuid.UUID) []catalog.PriceEntry {
	return v.History[productID]
}

// priceInput renders a stored price back into the decimal the form edits.
func priceInput(amount money.Amount) string {
	cents := amount.Cents
	if cents < 0 {
		cents = -cents
	}
	return strconv.FormatInt(cents/100, 10) + "." + fmt.Sprintf("%02d", cents%100)
}

// priceRows renders a price trail as table rows.
func priceRows(trail []catalog.PriceEntry) []templ.Component {
	rows := make([]templ.Component, 0, len(trail))
	for _, entry := range trail {
		rows = append(rows, adminPriceRow(entry))
	}
	return rows
}

// deltaTone colours a price move: up is the one worth noticing.
func deltaTone(entry catalog.PriceEntry) components.Tone {
	switch {
	case entry.Delta().Cents > 0:
		return components.ToneWarning
	case entry.Delta().Cents < 0:
		return components.ToneSuccess
	default:
		return components.ToneNeutral
	}
}

// changedByLabel names who moved the price, for the rows that predate the panel.
func changedByLabel(entry catalog.PriceEntry) string {
	if entry.ChangedBy == "" {
		return "before the panel"
	}
	return entry.ChangedBy
}

// AdminSettingsView is the settings form: what is stored now and what the
// environment would fall back to.
type AdminSettingsView struct {
	Values    settings.Values
	Defaults  settings.Values
	Error     string
	Saved     bool
	CSRFToken string
}

// ShippingHint spells the stored cents out as money, so nobody ships for $1500
// by typing the price in dollars.
func (v AdminSettingsView) ShippingHint() string {
	return "That is " + money.New(v.Values.ShippingCents, "USD").String() + " added to every order that has something in it."
}

// DefaultsHint says what the shop falls back to when a key is cleared.
func (v AdminSettingsView) DefaultsHint() string {
	return "Without a stored value the shop runs on its environment: " +
		money.New(v.Defaults.ShippingCents, "USD").String() + " delivery and a " +
		strconv.Itoa(v.Defaults.TTLMinutes()) + " minute reservation window."
}

// adminFact is one label/value line of a card panel.
type adminFact struct {
	Label string
	Value string
}
