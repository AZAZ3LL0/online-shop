package admin

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/google/uuid"

	"github.com/qzq-kiim/shop/internal/httpx/middleware"
	"github.com/qzq-kiim/shop/internal/httpx/reqctx"
	"github.com/qzq-kiim/shop/internal/storage/postgres"
	"github.com/qzq-kiim/shop/internal/telegram"
	"github.com/qzq-kiim/shop/web/templates"
	"github.com/qzq-kiim/shop/web/templates/pages"
)

// MiniPrefix is the Mini App mount point, tech.md §5.3. Everything under it is
// served by the admin handlers in the AdminMini layout.
const MiniPrefix = "/tgapp"

// miniSessionTTL is the Mini App session lifetime, tech.md §5.3. It is short on
// purpose: the Telegram client can re-authenticate silently at any time.
const miniSessionTTL = time.Hour

// CookieTheme carries the sanitised Telegram themeParams between the launch and
// the pages that style themselves from it.
const CookieTheme = "tg_theme"

// fieldInitData and fieldTheme are what the launch page posts to /tgapp/auth.
const (
	fieldInitData = "init_data"
	fieldTheme    = "theme"
)

// TelegramAuth is the Mini App admission control: verify the launch payload,
// check the allowlist, open a short session.
type TelegramAuth struct {
	admins    TelegramRepository
	cookies   themeSigner
	botToken  string
	allowlist map[int64]struct{}
	log       *slog.Logger
}

// TelegramRepository is the narrow storage the Mini App login needs.
type TelegramRepository interface {
	ByTelegramID(ctx context.Context, telegramID int64) (postgres.AdminUser, error)
	CreateSession(ctx context.Context, adminID uuid.UUID, ip, userAgent string, expiresAt time.Time) (uuid.UUID, error)
}

// NewTelegramAuth wires the Mini App entry point. allowed is
// ADMIN_TELEGRAM_IDS: when it is set it gates access before the database is
// touched at all, when it is empty admin_users.telegram_id is the only gate.
func NewTelegramAuth(admins TelegramRepository, signer themeSigner, botToken string, allowed []int64, log *slog.Logger) *TelegramAuth {
	list := make(map[int64]struct{}, len(allowed))
	for _, id := range allowed {
		list[id] = struct{}{}
	}
	return &TelegramAuth{admins: admins, cookies: signer, botToken: botToken, allowlist: list, log: log}
}

// Entry is GET /tgapp: the launch page. It carries no data of its own - it
// hands the payload Telegram put in the URL fragment to /tgapp/auth and follows
// the redirect. The fragment never reaches the server on its own.
func (a *TelegramAuth) Entry(w http.ResponseWriter, r *http.Request) {
	// The launch payload is single use, so this page is never cached: every
	// open of the Mini App has to re-run the exchange below.
	w.Header().Set("Cache-Control", "no-store")
	a.renderEntry(w, r)
}

// Auth is POST /tgapp/auth: initData in, a one hour admin session out. It is
// called by the launch page, so it answers JSON - the refusal wording stays on
// the server and the launch payload never turns into a navigation.
func (a *TelegramAuth) Auth(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		a.reject(w, r, err)
		return
	}
	data, err := telegram.VerifyInitData(
		r.PostForm.Get(fieldInitData), a.botToken, time.Now().UTC(), telegram.InitDataMaxAge)
	if err != nil {
		a.reject(w, r, err)
		return
	}
	if !a.allowed(data.User.ID) {
		a.reject(w, r, errors.New("telegram id is not on the allowlist"))
		return
	}
	admin, err := a.admins.ByTelegramID(r.Context(), data.User.ID)
	if err != nil {
		a.reject(w, r, err)
		return
	}

	sessionID, err := a.admins.CreateSession(
		r.Context(), admin.ID, middleware.ClientIP(r), r.UserAgent(), time.Now().Add(miniSessionTTL))
	if err != nil {
		a.log.Error("create mini app session failed",
			slog.String("request_id", reqctx.RequestID(r.Context())),
			slog.String("error", err.Error()))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	a.cookies.Set(w, middleware.CookieAdminSession, sessionID.String(), miniSessionTTL)

	// The colours are sanitised here, once, so no page has to trust the payload
	// again (tech.md §8, themeParams).
	if theme := telegram.ParseThemeParams(r.PostForm.Get(fieldTheme)).Encode(); theme != "" {
		a.cookies.Set(w, CookieTheme, theme, miniSessionTTL)
	}
	a.writeJSON(w, r, http.StatusOK, map[string]string{"redirect": MiniPrefix + "/"})
}

// allowed applies the ADMIN_TELEGRAM_IDS gate.
func (a *TelegramAuth) allowed(id int64) bool {
	if len(a.allowlist) == 0 {
		return true
	}
	_, ok := a.allowlist[id]
	return ok
}

// rejection is the single wording every refused launch gets. A stale auth_date,
// a forged hash and an account outside the allowlist are indistinguishable from
// the outside (tech.md §5.5, §9.13).
const rejection = "This Telegram account cannot open the admin panel."

// reject answers 403 and writes the real reason to the log only.
func (a *TelegramAuth) reject(w http.ResponseWriter, r *http.Request, cause error) {
	a.log.Warn("mini app launch rejected",
		slog.String("request_id", reqctx.RequestID(r.Context())),
		slog.String("error", cause.Error()))
	a.writeJSON(w, r, http.StatusForbidden, map[string]string{"error": rejection})
}

func (a *TelegramAuth) writeJSON(w http.ResponseWriter, r *http.Request, status int, payload map[string]string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		a.log.Error("write mini app response failed",
			slog.String("request_id", reqctx.RequestID(r.Context())),
			slog.String("error", err.Error()))
	}
}

// renderEntry serves the launch shell: no admin data, just the bootstrap that
// hands the fragment over.
func (a *TelegramAuth) renderEntry(w http.ResponseWriter, r *http.Request) {
	page := templates.Page{
		Title:     "QZQ admin",
		CSRFToken: reqctx.CSRFToken(r.Context()),
		Theme:     theme(a.cookies, r),
	}
	view := pages.TelegramLaunchView{
		AuthURL:   MiniPrefix + "/auth",
		CSRFToken: page.CSRFToken,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	ctx := templ.WithChildren(r.Context(), pages.TelegramLaunch(view))
	if err := templates.AdminMini(page).Render(ctx, w); err != nil {
		a.log.Error("render mini app launch failed",
			slog.String("request_id", reqctx.RequestID(r.Context())),
			slog.String("error", err.Error()))
	}
}

// IsMini reports whether the request is being served inside Telegram, which is
// the only thing that decides which layout the shared admin handlers render
// into (tech.md §5.3).
func IsMini(r *http.Request) bool {
	return r.URL.Path == MiniPrefix || strings.HasPrefix(r.URL.Path, MiniPrefix+"/")
}

// themeSigner is the cookie seam both the launch and the layout need.
type themeSigner interface {
	Set(w http.ResponseWriter, name, value string, ttl time.Duration)
	Get(r *http.Request, name string) (string, bool)
}

// theme reads the sanitised colour scheme back out of its cookie. It is parsed
// again rather than trusted: the cookie is signed, the values still have to be
// colours before they reach a stylesheet.
func theme(signer themeSigner, r *http.Request) telegram.ThemeParams {
	raw, ok := signer.Get(r, CookieTheme)
	if !ok {
		return nil
	}
	return telegram.ParseThemeParams(raw)
}
