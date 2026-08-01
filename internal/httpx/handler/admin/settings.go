package admin

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/qzq-kiim/shop/internal/domain/settings"
	"github.com/qzq-kiim/shop/internal/httpx/reqctx"
	"github.com/qzq-kiim/shop/web/templates"
	"github.com/qzq-kiim/shop/web/templates/pages"
)

// Settings shows the runtime parameters of the shop.
func (h *Handler) Settings(w http.ResponseWriter, r *http.Request) {
	values, err := h.settings.Values(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.renderSettings(w, r, http.StatusOK, values, "", r.URL.Query().Get("saved") != "")
}

// SettingsSave stores the whole set through the key-value repository. Nothing
// here gets a column of its own (tech.md §4).
func (h *Handler) SettingsSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	current, err := h.settings.Values(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}

	submitted := settings.Values{ShopPaused: r.PostForm.Get("shop_paused") != ""}
	shipping, shippingErr := strconv.ParseInt(strings.TrimSpace(r.PostForm.Get("shipping_cents")), 10, 64)
	minutes, ttlErr := strconv.Atoi(strings.TrimSpace(r.PostForm.Get("order_ttl_minutes")))
	if shippingErr != nil || ttlErr != nil {
		h.renderSettings(w, r, http.StatusUnprocessableEntity, current,
			"Shipping and the reservation window are whole numbers.", false)
		return
	}
	submitted.ShippingCents = shipping
	submitted.OrderTTL = time.Duration(minutes) * time.Minute

	err = h.settings.Save(r.Context(), submitted)
	switch {
	case err == nil:
		http.Redirect(w, r, "/admin/settings?saved=1", http.StatusSeeOther)
	case errors.Is(err, settings.ErrOutOfRange):
		// The bounds are the message: the operator gets told what is allowed.
		h.renderSettings(w, r, http.StatusUnprocessableEntity, submitted, rangeMessage(err), false)
	default:
		h.fail(w, r, err)
	}
}

func (h *Handler) renderSettings(w http.ResponseWriter, r *http.Request, status int, values settings.Values, message string, saved bool) {
	h.renderPageStatus(w, r, status, "Settings", templates.SectionSettings, pages.AdminSettings(pages.AdminSettingsView{
		Values:    values,
		Defaults:  h.settings.Defaults(),
		Error:     message,
		Saved:     saved,
		CSRFToken: reqctx.CSRFToken(r.Context()),
	}))
}

// rangeMessage turns the domain refusal into one sentence for the form.
func rangeMessage(err error) string {
	_, detail, found := strings.Cut(err.Error(), ": ")
	if !found {
		return "That value is out of range."
	}
	return strings.ToUpper(detail[:1]) + detail[1:] + "."
}
