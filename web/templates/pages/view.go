// Package pages holds the page-level templates. Every visual primitive they use
// comes from the components package.
package pages

import (
	"github.com/qzq-kiim/shop/internal/domain/cart"
	"github.com/qzq-kiim/shop/internal/domain/catalog"
	"github.com/qzq-kiim/shop/internal/money"
	"github.com/qzq-kiim/shop/web/templates/components"
)

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

// inStock reports whether any size of the product can still be bought.
func inStock(p catalog.Product) bool {
	for _, v := range p.Variants {
		if v.Available() > 0 {
			return true
		}
	}
	return false
}
