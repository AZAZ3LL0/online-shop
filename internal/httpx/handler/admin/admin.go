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
	"github.com/qzq-kiim/shop/internal/domain/analytics"
	"github.com/qzq-kiim/shop/internal/httpx/cookies"
	"github.com/qzq-kiim/shop/internal/httpx/middleware"
	"github.com/qzq-kiim/shop/internal/httpx/reqctx"
	"github.com/qzq-kiim/shop/internal/storage/postgres"
	"github.com/qzq-kiim/shop/web/templates"
	"github.com/qzq-kiim/shop/web/templates/pages"
)

// sessionTTL is the browser admin session lifetime, tech.md §8.5.
const sessionTTL = 12 * time.Hour

// Repository is the narrow admin storage the handlers need. ByTelegramID is the
// Mini App allowlist; the browser panel never calls it.
type Repository interface {
	ByLogin(ctx context.Context, login string) (postgres.AdminUser, error)
	ByTelegramID(ctx context.Context, telegramID int64) (postgres.AdminUser, error)
	CreateSession(ctx context.Context, adminID uuid.UUID, ip, userAgent string, expiresAt time.Time) (uuid.UUID, error)
	DeleteSession(ctx context.Context, id uuid.UUID) error
}

// Handler serves the admin panel.
type Handler struct {
	admins  Repository
	metrics analytics.MetricsRepository
	cookies *cookies.Signer
	log     *slog.Logger
}

// New wires the admin handler.
func New(admins Repository, metrics analytics.MetricsRepository, signer *cookies.Signer, log *slog.Logger) *Handler {
	return &Handler{admins: admins, metrics: metrics, cookies: signer, log: log}
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

// Dashboard is the landing page behind adminauth: the stat cards, the revenue
// chart with its price markers and the top of the traffic table (tech.md §8.1).
func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	view, ok := h.metricsView(w, r)
	if !ok {
		return
	}
	h.renderAdmin(w, r, "Admin - QZQ", pages.AdminDashboard(view))
}

// Analytics is the full report: the funnel and every traffic source (§8.2).
func (h *Handler) Analytics(w http.ResponseWriter, r *http.Request) {
	view, ok := h.metricsView(w, r)
	if !ok {
		return
	}
	h.renderAdmin(w, r, "Analytics - QZQ", pages.AdminAnalytics(view))
}

// metricsView loads the report both admin pages are built from. It answers the
// request itself and returns false when it could not.
func (h *Handler) metricsView(w http.ResponseWriter, r *http.Request) (pages.MetricsView, bool) {
	rng, err := h.parseRange(r)
	if err != nil {
		// A broken selector must not leave the operator on an error page: the
		// default period is reported instead, with the picker back at its start.
		rng, err = analytics.ParseRange("", "", "", "", time.Now().UTC())
		if err != nil {
			h.fail(w, r, err)
			return pages.MetricsView{}, false
		}
	}
	report, err := h.metrics.Metrics(r.Context(), rng)
	if err != nil {
		h.fail(w, r, err)
		return pages.MetricsView{}, false
	}
	return pages.MetricsView{Metrics: report, Path: r.URL.Path}, true
}

// renderAdmin writes one admin page into the layout the request asks for. This
// is the whole difference between the browser panel and the Mini App: the
// handlers, the queries and the view models above are the same (tech.md §5.3).
func (h *Handler) renderAdmin(w http.ResponseWriter, r *http.Request, title string, body templ.Component) {
	admin, _ := reqctx.AdminFrom(r.Context())
	page := templates.Page{
		Title:     title,
		CSRFToken: reqctx.CSRFToken(r.Context()),
		AdminUser: admin.Login,
	}
	if IsMini(r) {
		page.Theme = theme(h.cookies, r)
		h.render(w, r, templates.AdminMini(page).Render, body)
		return
	}
	h.render(w, r, templates.AdminWeb(page).Render, body)
}

// fail logs the cause and answers with a generic message, tech.md §9.13.
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	h.log.Error("admin request failed",
		slog.String("request_id", reqctx.RequestID(r.Context())),
		slog.String("path", r.URL.Path),
		slog.String("error", err.Error()))
	http.Error(w, "internal server error", http.StatusInternalServerError)
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

func (h *Handler) render(w http.ResponseWriter, r *http.Request, layout func(context.Context, io.Writer) error, body templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
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
