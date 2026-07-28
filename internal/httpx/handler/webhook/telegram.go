package webhook

import (
	"crypto/subtle"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/qzq-kiim/shop/internal/httpx/reqctx"
	"github.com/qzq-kiim/shop/internal/telegram"
)

// HeaderSecretToken is the header carrying the shared secret Telegram echoes on
// every delivery, tech.md §5.5. This is the header name, not the secret: the
// secret itself only ever comes from the environment.
//
//nolint:gosec // G101 matches the header name, there is no credential here.
const HeaderSecretToken = "X-Telegram-Bot-Api-Secret-Token"

// updateTimeout bounds the work one update may do, tech.md §16.2.
const updateTimeout = 10 * time.Second

// Telegram receives bot updates. Both secrets are checked before the body is
// parsed, and the update id is deduplicated before anything is acted on.
type Telegram struct {
	router     *telegram.Router
	repo       telegram.Repository
	secret     string
	pathSecret string
	log        *slog.Logger
}

// NewTelegram wires the bot webhook.
func NewTelegram(router *telegram.Router, repo telegram.Repository, secret, pathSecret string, log *slog.Logger) *Telegram {
	return &Telegram{router: router, repo: repo, secret: secret, pathSecret: pathSecret, log: log}
}

// Handle answers one update. Anything other than a failed secret check gets a
// 200: Telegram retries whatever it does not get an acknowledgement for, and a
// retry must never be able to multiply the effect of an update (tech.md §5.5).
func (h *Telegram) Handle(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := timeout(r, updateTimeout)
	defer cancel()

	// Both secrets are compared in constant time and before the body is read,
	// tech.md §9.5. An unconfigured bot rejects everything rather than opening
	// the endpoint to the world.
	if !h.authorized(r) {
		h.log.Warn("telegram update rejected",
			slog.String("request_id", reqctx.RequestID(ctx)))
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	update, err := telegram.ParseUpdate(body)
	if err != nil {
		// A body we cannot read will not get better on redelivery.
		h.log.Warn("telegram update unreadable",
			slog.String("request_id", reqctx.RequestID(ctx)),
			slog.String("error", err.Error()))
		ok(w)
		return
	}

	seen, err := h.repo.SeenUpdate(ctx, update.UpdateID)
	if err != nil {
		h.log.Error("record telegram update failed",
			slog.String("request_id", reqctx.RequestID(ctx)),
			slog.String("error", err.Error()))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if seen {
		h.log.Info("telegram update already handled",
			slog.String("request_id", reqctx.RequestID(ctx)),
			slog.Int64("update_id", update.UpdateID))
		ok(w)
		return
	}

	if err := h.router.Handle(ctx, update); err != nil {
		h.log.Error("handle telegram update failed",
			slog.String("request_id", reqctx.RequestID(ctx)),
			slog.Int64("update_id", update.UpdateID),
			slog.String("error", err.Error()))
	}
	ok(w)
}

// authorized checks the header secret and the path secret. Both are compared
// with subtle.ConstantTimeCompare so neither can be guessed by timing.
func (h *Telegram) authorized(r *http.Request) bool {
	if h.secret == "" || h.pathSecret == "" {
		return false
	}
	headerOK := subtle.ConstantTimeCompare([]byte(r.Header.Get(HeaderSecretToken)), []byte(h.secret)) == 1
	pathOK := subtle.ConstantTimeCompare([]byte(r.PathValue("secret")), []byte(h.pathSecret)) == 1
	return headerOK && pathOK
}

func ok(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok"))
}
