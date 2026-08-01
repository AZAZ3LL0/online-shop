// Package templates holds the layouts every page is rendered into.
package templates

import (
	"github.com/a-h/templ"

	"github.com/qzq-kiim/shop/internal/telegram"
)

// Page is the data every layout needs, independent of the concrete page.
type Page struct {
	Title     string
	CartCount int
	CSRFToken string
	IsDev     bool
	AdminUser string
	// Theme is the Telegram colour scheme of a Mini App request, empty
	// everywhere else. The browser layout ignores it.
	Theme telegram.ThemeParams
}

// ThemeStyle turns the Telegram colour scheme into the one <style> block the
// Mini App layout carries. The values were validated as hex colours before they
// were stored, and are validated again on the way out, so this is the only
// place templ.Raw is used on anything that came from outside (tech.md §9.2).
func (p Page) ThemeStyle() templ.Component {
	vars := p.Theme.Vars()
	if vars == "" {
		return templ.NopComponent
	}
	return templ.Raw("<style>:root{" + vars + "}</style>")
}
