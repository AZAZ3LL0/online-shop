// Package templates holds the layouts every page is rendered into.
package templates

import (
	"github.com/a-h/templ"

	"github.com/qzq-kiim/shop/internal/telegram"
)

// Slogan is the brand line of the shop, tech.md §19.6. It is written down once:
// the ribbon under the header and the footer both render this constant, and
// there is no second copy of the string anywhere in the project.
const Slogan = "qzq.kiim · КЕЛ, ШАЙ ІШЕМІЗ · ЧӘЙ · СӘЙ · ÇAY · ЧАЙ · ШАЙ · СAЙ · ЧАИ · ČAI · ЦӘ"

// Admin sections: the keys the layout highlights its navigation by.
const (
	SectionDashboard = "dashboard"
	SectionOrders    = "orders"
	SectionProducts  = "products"
	SectionAnalytics = "analytics"
	SectionSettings  = "settings"
)

// Page is the data every layout needs, independent of the concrete page.
type Page struct {
	Title     string
	CartCount int
	CSRFToken string
	IsDev     bool
	AdminUser string
	// AdminSection is the key of the admin page being shown, empty on the
	// storefront. The layout highlights the matching navigation entry.
	AdminSection string
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

// adminSection is one entry of the admin navigation.
type adminSection struct {
	Key   string
	Href  string
	Label string
}

// adminSections is the admin navigation, in the order of tech.md §5.3.
var adminSections = []adminSection{
	{Key: SectionDashboard, Href: "/admin", Label: "Dashboard"},
	{Key: SectionOrders, Href: "/admin/orders", Label: "Orders"},
	{Key: SectionProducts, Href: "/admin/products", Label: "Products"},
	{Key: SectionAnalytics, Href: "/admin/analytics", Label: "Analytics"},
	{Key: SectionSettings, Href: "/admin/settings", Label: "Settings"},
}

func adminLinkClasses(current bool) string {
	if current {
		return "border-b-2 border-neutral-900 pb-0.5 font-medium text-neutral-900"
	}
	return "text-neutral-500 hover:text-neutral-900"
}
