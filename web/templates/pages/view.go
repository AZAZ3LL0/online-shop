// Package pages holds the page-level templates. Every visual primitive they use
// comes from the components package.
package pages

import (
	"github.com/a-h/templ"

	"github.com/qzq-kiim/shop/internal/domain/cart"
	"github.com/qzq-kiim/shop/internal/domain/catalog"
	"github.com/qzq-kiim/shop/internal/money"
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
