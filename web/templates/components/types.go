// Package components holds every UI primitive of the shop. Markup that is not
// here is duplicated markup, tech.md §7.2.
package components

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/qzq-kiim/shop/internal/domain/order"
)

// Tone is the closed set of semantic colours used by badges, alerts and pills.
type Tone string

// Tones.
const (
	ToneNeutral Tone = "neutral"
	ToneSuccess Tone = "success"
	ToneWarning Tone = "warning"
	ToneDanger  Tone = "danger"
)

// ButtonVariant is the closed set of button shapes.
type ButtonVariant string

// Button variants.
const (
	ButtonPrimary   ButtonVariant = "primary"
	ButtonSecondary ButtonVariant = "secondary"
	ButtonGhost     ButtonVariant = "ghost"
	ButtonDanger    ButtonVariant = "danger"
)

// Option is one entry of a Select.
type Option struct {
	Value    string
	Label    string
	Note     string
	Disabled bool
}

func toneClasses(t Tone) string {
	switch t {
	case ToneSuccess:
		return "bg-emerald-50 text-emerald-800 border-emerald-200"
	case ToneWarning:
		return "bg-amber-50 text-amber-900 border-amber-200"
	case ToneDanger:
		return "bg-rose-50 text-rose-800 border-rose-200"
	default:
		return "bg-neutral-100 text-neutral-800 border-neutral-200"
	}
}

func buttonClasses(v ButtonVariant) string {
	base := "inline-flex items-center justify-center gap-2 px-5 py-3 text-sm font-medium " +
		"tracking-wide transition-colors disabled:cursor-not-allowed disabled:opacity-40"
	switch v {
	case ButtonSecondary:
		return base + " border border-neutral-900 text-neutral-900 hover:bg-neutral-900 hover:text-white"
	case ButtonGhost:
		return base + " text-neutral-600 hover:text-neutral-900"
	case ButtonDanger:
		return base + " bg-rose-600 text-white hover:bg-rose-700"
	default:
		return base + " bg-neutral-900 text-white hover:bg-neutral-700"
	}
}

// StatusTone maps an order status onto a tone so status colours are defined once.
func StatusTone(s order.Status) Tone {
	switch s {
	case order.StatusPaid, order.StatusShipped, order.StatusDelivered:
		return ToneSuccess
	case order.StatusAwaitingPayment, order.StatusCreated:
		return ToneWarning
	case order.StatusExpired, order.StatusCancelled, order.StatusRefunded:
		return ToneDanger
	default:
		return ToneNeutral
	}
}

func inputClasses(hasError bool) string {
	base := "w-full border px-3 py-2 text-sm outline-none focus:border-neutral-900"
	if hasError {
		return base + " border-rose-400"
	}
	return base + " border-neutral-300"
}

// pageURL appends the page number to a base URL without breaking its query.
func pageURL(baseURL string, page int) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return baseURL
	}
	q := u.Query()
	q.Set("page", strconv.Itoa(page))
	u.RawQuery = q.Encode()
	return u.String()
}

func pageRange(page, totalPages int) []int {
	if totalPages < 1 {
		return nil
	}
	const window = 2
	from := max(page-window, 1)
	to := min(page+window, totalPages)
	pages := make([]int, 0, to-from+1)
	for p := from; p <= to; p++ {
		pages = append(pages, p)
	}
	return pages
}

func counterLabel(current, maxRunes int) string {
	return fmt.Sprintf("%d/%d", current, maxRunes)
}

// textareaState builds the Alpine state for Textarea. The counter counts runes,
// not bytes, because every user-facing limit in this project is in runes.
func textareaState(value string, maxRunes int) string {
	state, err := json.Marshal(map[string]any{"text": value, "max": maxRunes})
	if err != nil {
		return "{ text: '', max: 0 }"
	}
	return string(state)
}
