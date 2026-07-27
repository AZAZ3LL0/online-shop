package handler

import (
	"html/template"
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

// Templates are parsed once at startup and cached. Each page template is
// combined with the layout to avoid {{define "content"}} conflicts that
// would arise from parsing every page into a single set.
var (
	pageTemplates    = map[string]*template.Template{}
	partialTemplates = map[string]*template.Template{}
)

// InitTemplates precompiles all page and partial templates from dir.
// It must be called once before the server starts handling requests.
func InitTemplates(dir string) error {
	layout := filepath.Join(dir, "layout.html")

	pages, err := filepath.Glob(filepath.Join(dir, "*.html"))
	if err != nil {
		return err
	}
	for _, page := range pages {
		name := filepath.Base(page)
		if name == "layout.html" {
			continue
		}
		t, err := template.New("").ParseFiles(layout, page)
		if err != nil {
			return err
		}
		pageTemplates[name] = t
	}

	partials, err := filepath.Glob(filepath.Join(dir, "partials", "*.html"))
	if err != nil {
		return err
	}
	for _, partial := range partials {
		name := filepath.Base(partial)
		t, err := template.New("").ParseFiles(partial)
		if err != nil {
			return err
		}
		partialTemplates[name] = t
	}
	return nil
}

func render(c *gin.Context, status int, page string, data interface{}) {
	// Make the CSRF token available to every page so forms can embed it as a
	// hidden field (see middleware.CSRF).
	if h, ok := data.(gin.H); ok {
		if _, exists := h["CSRFToken"]; !exists {
			if tok, found := c.Get("csrf_token"); found {
				h["CSRFToken"] = tok
			}
		}
	}

	t, ok := pageTemplates[page]
	if !ok {
		c.String(http.StatusInternalServerError, "unknown template: %s", page)
		return
	}
	c.Status(status)
	if err := t.ExecuteTemplate(c.Writer, "layout", data); err != nil {
		c.String(http.StatusInternalServerError, "render error: %v", err)
	}
}

// renderPartial renders a standalone partial template (no layout).
func renderPartial(c *gin.Context, status int, partial string, data interface{}) {
	t, ok := partialTemplates[partial]
	if !ok {
		c.String(http.StatusInternalServerError, "unknown partial: %s", partial)
		return
	}
	c.Status(status)
	if err := t.ExecuteTemplate(c.Writer, partial, data); err != nil {
		c.String(http.StatusInternalServerError, "render error: %v", err)
	}
}
