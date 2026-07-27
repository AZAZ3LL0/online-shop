package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/qzq-kiim/shop/internal/httpx/cookies"
	"github.com/qzq-kiim/shop/internal/httpx/reqctx"
	"github.com/qzq-kiim/shop/internal/storage/postgres"
)

// CookieAdminSession holds the admin session id.
const CookieAdminSession = "admin_session"

// SessionReader is the narrow view of the admin repository this layer needs.
type SessionReader interface {
	Session(ctx context.Context, id uuid.UUID) (postgres.AdminSession, error)
}

// AdminAuth rejects every admin request without a live session. Handlers never
// repeat this check.
func AdminAuth(sessions SessionReader, signer *cookies.Signer, loginPath string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, ok := signer.Get(r, CookieAdminSession)
			if !ok {
				redirectToLogin(w, r, loginPath)
				return
			}
			id, err := uuid.Parse(raw)
			if err != nil {
				redirectToLogin(w, r, loginPath)
				return
			}
			session, err := sessions.Session(r.Context(), id)
			if err != nil {
				redirectToLogin(w, r, loginPath)
				return
			}
			ctx := reqctx.WithAdmin(r.Context(), reqctx.Admin{
				SessionID: session.ID,
				AdminID:   session.AdminID,
				Login:     session.Login,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func redirectToLogin(w http.ResponseWriter, r *http.Request, loginPath string) {
	if r.Method != http.MethodGet {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	http.Redirect(w, r, loginPath, http.StatusSeeOther)
}
