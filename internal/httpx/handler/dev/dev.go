// Package dev holds handlers that only exist in a development build.
package dev

import (
	"log/slog"
	"net/http"

	"github.com/a-h/templ"

	"github.com/qzq-kiim/shop/internal/httpx/reqctx"
	"github.com/qzq-kiim/shop/web/templates"
	"github.com/qzq-kiim/shop/web/templates/pages"
)

// Handler serves the development-only routes.
type Handler struct {
	log *slog.Logger
}

// New wires the dev handler.
func New(log *slog.Logger) *Handler { return &Handler{log: log} }

// KitchenSink renders every UI primitive on one page.
func (h *Handler) KitchenSink(w http.ResponseWriter, r *http.Request) {
	page := templates.Page{
		Title:     "Kitchen sink",
		CSRFToken: reqctx.CSRFToken(r.Context()),
		IsDev:     true,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	ctx := templ.WithChildren(r.Context(), pages.KitchenSink())
	if err := templates.Shop(page).Render(ctx, w); err != nil {
		h.log.Error("render kitchen sink failed",
			slog.String("request_id", reqctx.RequestID(r.Context())),
			slog.String("error", err.Error()))
	}
}
