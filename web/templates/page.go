// Package templates holds the layouts every page is rendered into.
package templates

// Page is the data every layout needs, independent of the concrete page.
type Page struct {
	Title     string
	CartCount int
	CSRFToken string
	IsDev     bool
	AdminUser string
}
