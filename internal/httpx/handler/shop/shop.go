// Package shop holds the public storefront handlers.
package shop

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/qzq-kiim/shop/internal/domain/analytics"
	"github.com/qzq-kiim/shop/internal/domain/cart"
	"github.com/qzq-kiim/shop/internal/domain/catalog"
	"github.com/qzq-kiim/shop/internal/domain/order"
	"github.com/qzq-kiim/shop/internal/httpx/cookies"
	"github.com/qzq-kiim/shop/internal/httpx/reqctx"
	"github.com/qzq-kiim/shop/internal/payments/nowpayments"
	"github.com/qzq-kiim/shop/web/templates"
	"github.com/qzq-kiim/shop/web/templates/pages"
)

// CookieCart holds the signed cart id, tech.md §4 (carts.id).
const CookieCart = "cart_id"

// cartTTL keeps an abandoned cart addressable for a month.
const cartTTL = 30 * 24 * time.Hour

// Deps is everything the storefront needs, injected by the router.
type Deps struct {
	Catalog   catalog.Repository
	Carts     *cart.Service
	Orders    *order.Service
	Payments  nowpayments.Provider
	Analytics analytics.Repository
	Cookies   *cookies.Signer
	Log       *slog.Logger
	BaseURL   string
	// BotUsername drives the Telegram entry point; empty hides it entirely.
	BotUsername string
	OrderTTL    time.Duration
	IsDev       bool
}

// Handler serves the storefront.
type Handler struct {
	catalog     catalog.Repository
	carts       *cart.Service
	orders      *order.Service
	payments    nowpayments.Provider
	analytics   analytics.Repository
	cookies     *cookies.Signer
	log         *slog.Logger
	baseURL     string
	botUsername string
	orderTTL    time.Duration
	isDev       bool
}

// New wires the storefront handler.
func New(d Deps) *Handler {
	return &Handler{
		catalog:     d.Catalog,
		carts:       d.Carts,
		orders:      d.Orders,
		payments:    d.Payments,
		analytics:   d.Analytics,
		cookies:     d.Cookies,
		log:         d.Log,
		baseURL:     d.BaseURL,
		botUsername: d.BotUsername,
		orderTTL:    d.OrderTTL,
		isDev:       d.IsDev,
	}
}

// Home renders the three product cards and the cart panel.
func (h *Handler) Home(w http.ResponseWriter, r *http.Request) {
	products, err := h.catalog.ListActive(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	cartID, err := h.ensureCart(w, r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	view, err := h.cartView(r, cartID)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	page := templates.Page{
		Title:     "QZQ - merch",
		CartCount: view.Cart.Count(),
		CSRFToken: reqctx.CSRFToken(r.Context()),
		IsDev:     h.isDev,
	}
	h.recordPageView(r)
	body := pages.Home(pages.HomeView{Products: products, Cart: view, CSRFToken: page.CSRFToken})
	h.render(w, r, templates.Shop(page).Render, body)
}

// Product renders one model: both covers, the size run and the add form.
func (h *Handler) Product(w http.ResponseWriter, r *http.Request) {
	product, err := h.catalog.BySlug(r.Context(), r.PathValue("slug"))
	if err != nil {
		if errors.Is(err, catalog.ErrNotFound) {
			h.notFound(w, r)
			return
		}
		h.fail(w, r, err)
		return
	}
	cartID, err := h.ensureCart(w, r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	view, err := h.cartView(r, cartID)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	page := templates.Page{
		Title:     product.Title + " - QZQ",
		CartCount: view.Cart.Count(),
		CSRFToken: reqctx.CSRFToken(r.Context()),
		IsDev:     h.isDev,
	}
	h.recordPageView(r)
	h.recordProductView(r, product.ID, product.Slug)
	body := pages.Product(pages.ProductView{Product: product, Cart: view, CSRFToken: page.CSRFToken})
	h.render(w, r, templates.Shop(page).Render, body)
}

// CartPage renders the standalone cart page.
func (h *Handler) CartPage(w http.ResponseWriter, r *http.Request) {
	cartID, err := h.ensureCart(w, r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	view, err := h.cartView(r, cartID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	page := templates.Page{
		Title:     "Cart - QZQ",
		CartCount: view.Cart.Count(),
		CSRFToken: reqctx.CSRFToken(r.Context()),
		IsDev:     h.isDev,
	}
	h.recordPageView(r)
	h.render(w, r, templates.Shop(page).Render, pages.CartPage(view))
}

// AddItem adds a variant to the cart and answers with the cart fragment.
func (h *Handler) AddItem(w http.ResponseWriter, r *http.Request) {
	cartID, err := h.ensureCart(w, r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		h.fragmentError(w, r, cartID, http.StatusBadRequest, "Malformed request.")
		return
	}
	variantID, err := uuid.Parse(r.PostForm.Get("variant_id"))
	if err != nil {
		h.fragmentError(w, r, cartID, http.StatusBadRequest, "Pick a size first.")
		return
	}
	qty, err := strconv.Atoi(r.PostForm.Get("qty"))
	if err != nil {
		h.fragmentError(w, r, cartID, http.StatusUnprocessableEntity, "Quantity must be a number.")
		return
	}
	if err := h.carts.Add(r.Context(), cartID, variantID, qty); err != nil {
		h.fragmentFailure(w, r, cartID, err)
		return
	}
	h.recordEvent(r, analytics.EventAddToCart, payload(map[string]any{
		"variant_id": variantID.String(),
		"qty":        qty,
	}))
	h.writeFragment(w, r, cartID, http.StatusOK, "")
}

// UpdateItem changes the quantity of one cart line.
func (h *Handler) UpdateItem(w http.ResponseWriter, r *http.Request) {
	cartID, err := h.ensureCart(w, r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	itemID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		h.fragmentError(w, r, cartID, http.StatusNotFound, "That item is no longer in your cart.")
		return
	}
	if err := r.ParseForm(); err != nil {
		h.fragmentError(w, r, cartID, http.StatusBadRequest, "Malformed request.")
		return
	}
	qty, err := strconv.Atoi(r.PostForm.Get("qty"))
	if err != nil {
		h.fragmentError(w, r, cartID, http.StatusUnprocessableEntity, "Quantity must be a number.")
		return
	}
	if err := h.carts.SetQty(r.Context(), cartID, itemID, qty); err != nil {
		h.fragmentFailure(w, r, cartID, err)
		return
	}
	h.writeFragment(w, r, cartID, http.StatusOK, "")
}

// RemoveItem drops one cart line.
func (h *Handler) RemoveItem(w http.ResponseWriter, r *http.Request) {
	cartID, err := h.ensureCart(w, r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	itemID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		h.fragmentError(w, r, cartID, http.StatusNotFound, "That item is no longer in your cart.")
		return
	}
	if err := h.carts.Remove(r.Context(), cartID, itemID); err != nil {
		h.fragmentFailure(w, r, cartID, err)
		return
	}
	h.recordEvent(r, analytics.EventRemoveFromCart, payload(map[string]any{"item_id": itemID.String()}))
	h.writeFragment(w, r, cartID, http.StatusOK, "")
}
