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

// OrnamentKind is the closed set of Kazakh ornaments the UI draws (tech.md
// §19.4). They are decoration only: nothing is ever said by an ornament alone.
type OrnamentKind string

// Ornaments.
const (
	// OrnamentBand is the horizontal ram's-horn ribbon used as a divider.
	OrnamentBand OrnamentKind = "band"
	// OrnamentRosette is the single qoshqar muyiz rosette used as an accent.
	OrnamentRosette OrnamentKind = "rosette"
	// OrnamentRule is the thin gold hairline with a diamond in the middle.
	OrnamentRule OrnamentKind = "rule"
)

// Option is one entry of a Select.
type Option struct {
	Value    string
	Label    string
	Note     string
	Disabled bool
}

// Colours are named by token, never literally: the same class renders light in
// the browser admin and dark on the storefront, because the scope on <body>
// decides what the token means (tech.md §19.2).
func toneClasses(t Tone) string {
	switch t {
	case ToneSuccess:
		return "bg-success-bg text-success-ink border-success-hair"
	case ToneWarning:
		return "bg-warning-bg text-warning-ink border-warning-hair"
	case ToneDanger:
		return "bg-danger-bg text-danger-ink border-danger-hair"
	default:
		return "bg-surface-2 text-ink border-hair"
	}
}

func buttonClasses(v ButtonVariant) string {
	base := "inline-flex items-center justify-center gap-2 px-5 py-3 text-sm font-medium " +
		"tracking-wide transition-colors duration-200 ease-soft " +
		"disabled:cursor-not-allowed disabled:opacity-40"
	switch v {
	case ButtonSecondary:
		return base + " border border-accent text-accent hover:bg-accent hover:text-accent-fg"
	case ButtonGhost:
		return base + " text-ink-muted hover:text-ink"
	case ButtonDanger:
		return base + " bg-danger-solid text-danger-solid-fg hover:opacity-90"
	default:
		return base + " bg-accent text-accent-fg hover:opacity-90"
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
	base := "w-full border bg-surface px-3 py-2 text-sm text-ink outline-none " +
		"transition-colors duration-200 ease-soft placeholder:text-ink-faint focus:border-accent"
	if hasError {
		return base + " border-danger-edge"
	}
	return base + " border-hair-strong"
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
