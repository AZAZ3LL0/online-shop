package admin

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/qzq-kiim/shop/internal/domain/catalog"
	"github.com/qzq-kiim/shop/internal/httpx/reqctx"
	"github.com/qzq-kiim/shop/web/templates"
	"github.com/qzq-kiim/shop/web/templates/pages"
)

// Products renders the models with their sizes and their price trail.
func (h *Handler) Products(w http.ResponseWriter, r *http.Request) {
	h.renderProducts(w, r, http.StatusOK, pages.AdminNotice{})
}

// ProductPrice records a price change. The reason is mandatory, and the change
// never touches an order that was already placed: those carry their own snapshot
// in order_items (tech.md §8.3).
func (h *Handler) ProductPrice(w http.ResponseWriter, r *http.Request) {
	id, ok := h.productID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	admin, _ := reqctx.AdminFrom(r.Context())

	price, err := catalog.ParsePrice(r.PostForm.Get("price"), h.currency)
	if err != nil {
		h.refuseEdit(w, r, id, "Enter the price as a decimal, for example 35.00.")
		return
	}
	err = h.products.ChangePrice(r.Context(), catalog.PriceChange{
		ProductID: id,
		NewPrice:  price,
		Reason:    r.PostForm.Get("reason"),
		ChangedBy: admin.AdminID,
	})
	switch {
	case err == nil:
		h.afterEdit(w, r, id, "price")
	case errors.Is(err, catalog.ErrReasonRequired):
		h.refuseEdit(w, r, id, "Say why the price changes, between 3 and 200 characters.")
	case errors.Is(err, catalog.ErrPriceOutOfRange):
		h.refuseEdit(w, r, id, "That price is out of range.")
	case errors.Is(err, catalog.ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	default:
		h.fail(w, r, err)
	}
}

// ProductStock replaces the stock of one size.
func (h *Handler) ProductStock(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.productID(w, r); !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	variantID, err := uuid.Parse(strings.TrimSpace(r.PostForm.Get("variant_id")))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	productID, err := h.products.ProductOfVariant(r.Context(), variantID)
	if err != nil {
		if errors.Is(err, catalog.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		h.fail(w, r, err)
		return
	}

	stock, convErr := strconv.Atoi(strings.TrimSpace(r.PostForm.Get("stock")))
	if convErr != nil {
		h.refuseEdit(w, r, productID, "Enter the stock as a whole number.")
		return
	}
	err = h.products.SetStock(r.Context(), variantID, stock)
	switch {
	case err == nil:
		h.afterEdit(w, r, productID, "stock")
	case errors.Is(err, catalog.ErrStockOutOfRange):
		h.refuseEdit(w, r, productID, "That stock is out of range.")
	case errors.Is(err, catalog.ErrStockBelowReserved):
		h.refuseEdit(w, r, productID, "That size has units reserved in a live checkout: the stock cannot go below them.")
	case errors.Is(err, catalog.ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	default:
		h.fail(w, r, err)
	}
}

// afterEdit sends the browser back to the page it submitted from, so a reload
// never repeats the edit.
func (h *Handler) afterEdit(w http.ResponseWriter, r *http.Request, productID uuid.UUID, what string) {
	http.Redirect(w, r, "/admin/products?saved="+what+"#product-"+productID.String(), http.StatusSeeOther)
}

// refuseEdit answers a rejected edit with the page and the reason on it.
func (h *Handler) refuseEdit(w http.ResponseWriter, r *http.Request, productID uuid.UUID, message string) {
	h.renderProducts(w, r, http.StatusUnprocessableEntity, pages.AdminNotice{
		ProductID: productID,
		Error:     message,
	})
}

// renderProducts draws the product page with an optional notice on it.
func (h *Handler) renderProducts(w http.ResponseWriter, r *http.Request, status int, notice pages.AdminNotice) {
	products, err := h.products.Products(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}

	view := pages.AdminProductsView{
		Products:  products,
		History:   make(map[uuid.UUID][]catalog.PriceEntry, len(products)),
		Notice:    notice,
		Saved:     r.URL.Query().Get("saved"),
		CSRFToken: reqctx.CSRFToken(r.Context()),
	}
	for _, p := range products {
		trail, err := h.products.PriceHistory(r.Context(), p.ID)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		view.History[p.ID] = trail
	}

	h.renderPageStatus(w, r, status, "Products", templates.SectionProducts, pages.AdminProducts(view))
}

// productID reads the product id out of the path.
func (h *Handler) productID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return uuid.Nil, false
	}
	return id, true
}
