package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/qzq-kiim/shop/internal/domain/analytics"
	"github.com/qzq-kiim/shop/internal/httpx/cookies"
	"github.com/qzq-kiim/shop/internal/httpx/reqctx"
)

// Attribution cookies, tech.md §5.6.
const (
	CookieVisitor    = "vid"
	CookieSession    = "sid"
	CookieFirstTouch = "ft"
)

// Cookie lifetimes, tech.md §5.6.
const (
	visitorTTL = 365 * 24 * time.Hour
	sessionTTL = 30 * time.Minute
)

// botAgents are the obvious crawlers; matched case-insensitively on User-Agent.
var botAgents = []string{
	"bot", "crawler", "spider", "curl", "wget", "headlesschrome",
	"python-requests", "go-http-client", "facebookexternalhit", "slurp",
}

// Attribution issues the visitor and session cookies, hands the attribution
// snapshot to the handlers and records a visit when a new session starts.
//
// Every method is attributed, because the cart and the checkout are form posts
// and an order without its visitor is an order that cannot be reported on. Only
// a page request writes to visits: a post continues the session its form was
// rendered in (tech.md §5.6).
func Attribution(repo analytics.Repository, signer *cookies.Signer, log *slog.Logger, skip ...string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if hasPrefix(r.URL.Path, skip) {
				next.ServeHTTP(w, r)
				return
			}
			isPageRequest := r.Method == http.MethodGet || r.Method == http.MethodHead

			visitorID, isNewVisitor := readOrIssueUUID(w, r, signer, CookieVisitor, visitorTTL)
			sessionID, isNewSession := readOrIssueUUID(w, r, signer, CookieSession, sessionTTL)
			// Sliding window: every request pushes the session cookie forward.
			if !isNewSession {
				signer.Set(w, CookieSession, sessionID.String(), sessionTTL)
			}

			touch := analytics.Touch{
				VisitorID:   visitorID,
				UTMSource:   r.URL.Query().Get("utm_source"),
				UTMMedium:   r.URL.Query().Get("utm_medium"),
				UTMCampaign: r.URL.Query().Get("utm_campaign"),
				UTMContent:  r.URL.Query().Get("utm_content"),
				UTMTerm:     r.URL.Query().Get("utm_term"),
				Referrer:    r.Referer(),
				LandingPath: r.URL.Path,
			}
			isBot := looksLikeBot(r.UserAgent())

			if isNewVisitor {
				if encoded, err := json.Marshal(touch); err == nil {
					signer.Set(w, CookieFirstTouch, string(encoded), visitorTTL)
				}
			}

			if isNewSession && isPageRequest {
				visit := analytics.Visit{
					SessionID: sessionID,
					Touch:     touch,
					UserAgent: r.UserAgent(),
					IsBot:     isBot,
				}
				if err := repo.RecordVisit(r.Context(), visit); err != nil {
					log.Error("record visit failed",
						slog.String("request_id", reqctx.RequestID(r.Context())),
						slog.String("error", err.Error()))
				}
			}

			ctx := reqctx.WithTouch(r.Context(), touch)
			ctx = reqctx.WithSessionID(ctx, sessionID)
			ctx = reqctx.WithBot(ctx, isBot)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// FirstTouch reads the first-touch snapshot from the signed cookie.
func FirstTouch(r *http.Request, signer *cookies.Signer) (analytics.Touch, bool) {
	raw, ok := signer.Get(r, CookieFirstTouch)
	if !ok {
		return analytics.Touch{}, false
	}
	var t analytics.Touch
	if err := json.Unmarshal([]byte(raw), &t); err != nil {
		return analytics.Touch{}, false
	}
	return t, true
}

func readOrIssueUUID(w http.ResponseWriter, r *http.Request, signer *cookies.Signer, name string, ttl time.Duration) (uuid.UUID, bool) {
	if raw, ok := signer.Get(r, name); ok {
		if id, err := uuid.Parse(raw); err == nil {
			return id, false
		}
	}
	id := uuid.New()
	signer.Set(w, name, id.String(), ttl)
	return id, true
}

func looksLikeBot(userAgent string) bool {
	ua := strings.ToLower(userAgent)
	if ua == "" {
		return true
	}
	for _, marker := range botAgents {
		if strings.Contains(ua, marker) {
			return true
		}
	}
	return false
}
