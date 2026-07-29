// Package admin holds the administration handlers. The same handlers serve the
// browser and the Telegram Mini App; only the layout differs.
package admin

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/a-h/templ"
	"github.com/google/uuid"

	"github.com/qzq-kiim/shop/internal/auth"
	"github.com/qzq-kiim/shop/internal/domain/catalog"
	"github.com/qzq-kiim/shop/internal/domain/order"
	"github.com/qzq-kiim/shop/internal/httpx/cookies"
	"github.com/qzq-kiim/shop/internal/httpx/middleware"
	"github.com/qzq-kiim/shop/internal/httpx/reqctx"
	"github.com/qzq-kiim/shop/internal/storage/postgres"
	"github.com/qzq-kiim/shop/web/templates"
	"github.com/qzq-kiim/shop/web/templates/pages"
)

// sessionTTL is the browser admin session lifetime, tech.md §8.5.
const sessionTTL = 12 * time.Hour

// Repository is the narrow admin storage the handlers need.
type Repository interface {
	ByLogin(ctx context.Context, login string) (postgres.AdminUser, error)
	CreateSession(ctx context.Context, adminID uuid.UUID, ip, userAgent string, expiresAt time.Time) (uuid.UUID, error)
	DeleteSession(ctx context.Context, id uuid.UUID) error
}

// Deps is everything the admin panel needs, injected by the router.
type Deps struct {
	Admins   Repository
	Orders   *order.AdminService
	Catalog  catalog.Repository
	Payments PaymentLog
	Links    ChatLinks
	Cookies  *cookies.Signer
	Log      *slog.Logger
}

// Handler serves the admin panel.
type Handler struct {
	admins   Repository
	orders   *order.AdminService
	catalog  catalog.Repository
	payments PaymentLog
	links    ChatLinks
	cookies  *cookies.Signer
	log      *slog.Logger
}

// New wires the admin handler.
func New(d Deps) *Handler {
	return &Handler{
		admins:   d.Admins,
		orders:   d.Orders,
		catalog:  d.Catalog,
		payments: d.Payments,
		links:    d.Links,
		cookies:  d.Cookies,
		log:      d.Log,
	}
}

// LoginForm renders the sign-in page.
func (h *Handler) LoginForm(w http.ResponseWriter, r *http.Request) {
	h.renderLogin(w, r, http.StatusOK, pages.LoginView{CSRFToken: reqctx.CSRFToken(r.Context())})
}

// Login checks the credentials and opens a session.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderLogin(w, r, http.StatusBadRequest, pages.LoginView{
			CSRFToken: reqctx.CSRFToken(r.Context()),
			Error:     "Malformed request.",
		})
		return
	}
	login := r.PostForm.Get("login")
	password := r.PostForm.Get("password")

	admin, err := h.admins.ByLogin(r.Context(), login)
	if err != nil {
		// The same answer for an unknown login and a wrong password: the form
		// must not tell an attacker which logins exist.
		h.rejectLogin(w, r, login, err)
		return
	}
	if err := auth.Verify(password, admin.PasswordHash); err != nil {
		h.rejectLogin(w, r, login, err)
		return
	}

	sessionID, err := h.admins.CreateSession(r.Context(), admin.ID, clientIP(r), r.UserAgent(), time.Now().Add(sessionTTL))
	if err != nil {
		h.log.Error("create admin session failed",
			slog.String("request_id", reqctx.RequestID(r.Context())),
			slog.String("error", err.Error()))
		h.renderLogin(w, r, http.StatusInternalServerError, pages.LoginView{
			CSRFToken: reqctx.CSRFToken(r.Context()),
			Login:     login,
			Error:     "Could not sign you in. Try again.",
		})
		return
	}
	h.cookies.Set(w, middleware.CookieAdminSession, sessionID.String(), sessionTTL)
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

// Logout ends the current session.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if admin, ok := reqctx.AdminFrom(r.Context()); ok {
		if err := h.admins.DeleteSession(r.Context(), admin.SessionID); err != nil {
			h.log.Error("delete admin session failed",
				slog.String("request_id", reqctx.RequestID(r.Context())),
				slog.String("error", err.Error()))
		}
	}
	h.cookies.Clear(w, middleware.CookieAdminSession)
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}

// Dashboard is the landing page behind adminauth. Its panels arrive with S6.
func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	h.renderPage(w, r, "Admin", templates.SectionDashboard, pages.AdminDashboard())
}

// renderPage writes one admin page: the shell around the body, with the
// navigation entry of section highlighted.
func (h *Handler) renderPage(w http.ResponseWriter, r *http.Request, title, section string, body templ.Component) {
	h.renderPageStatus(w, r, http.StatusOK, title, section, body)
}

// renderPageStatus is renderPage with an explicit status code, for the pages
// that answer a rejected form.
func (h *Handler) renderPageStatus(w http.ResponseWriter, r *http.Request, status int, title, section string, body templ.Component) {
	admin, _ := reqctx.AdminFrom(r.Context())
	page := templates.Page{
		Title:        title + " - QZQ admin",
		CSRFToken:    reqctx.CSRFToken(r.Context()),
		AdminUser:    admin.Login,
		AdminSection: section,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	h.renderBody(w, r, templates.AdminWeb(page).Render, body)
}

func (h *Handler) rejectLogin(w http.ResponseWriter, r *http.Request, login string, cause error) {
	if !errors.Is(cause, auth.ErrMismatch) && !errors.Is(cause, postgres.ErrNoAdmin) {
		h.log.Error("admin login failed",
			slog.String("request_id", reqctx.RequestID(r.Context())),
			slog.String("error", cause.Error()))
	}
	h.renderLogin(w, r, http.StatusUnauthorized, pages.LoginView{
		CSRFToken: reqctx.CSRFToken(r.Context()),
		Login:     login,
		Error:     "Wrong login or password.",
	})
}

func (h *Handler) renderLogin(w http.ResponseWriter, r *http.Request, status int, view pages.LoginView) {
	page := templates.Page{Title: "Admin sign in", CSRFToken: view.CSRFToken}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	ctx := templ.WithChildren(r.Context(), pages.AdminLogin(view))
	if err := templates.AdminWeb(page).Render(ctx, w); err != nil {
		h.log.Error("render login failed",
			slog.String("request_id", reqctx.RequestID(r.Context())),
			slog.String("error", err.Error()))
	}
}

func (h *Handler) renderBody(w http.ResponseWriter, r *http.Request, layout func(context.Context, io.Writer) error, body templ.Component) {
	ctx := templ.WithChildren(r.Context(), body)
	if err := layout(ctx, w); err != nil {
		h.log.Error("render admin page failed",
			slog.String("request_id", reqctx.RequestID(r.Context())),
			slog.String("error", err.Error()))
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return ""
	}
	return host
}

// fail logs the cause and answers with a generic message: nothing internal
// leaves the panel (tech.md §9.13).
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	h.log.Error("admin request failed",
		slog.String("request_id", reqctx.RequestID(r.Context())),
		slog.String("path", r.URL.Path),
		slog.String("error", err.Error()))
	http.Error(w, "internal server error", http.StatusInternalServerError)
}
