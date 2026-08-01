// Package reqctx carries per-request values that middleware produces and
// handlers consume, without any of them reaching for a global.
package reqctx

import (
	"context"

	"github.com/google/uuid"

	"github.com/qzq-kiim/shop/internal/domain/analytics"
)

type key int

const (
	keyRequestID key = iota
	keyTouch
	keySessionID
	keyCSRFToken
	keyAdmin
	keyBot
)

// Admin identifies the signed-in administrator of the current request.
type Admin struct {
	SessionID uuid.UUID
	AdminID   uuid.UUID
	Login     string
}

// WithRequestID stores the request id.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyRequestID, id)
}

// RequestID returns the request id, empty when unset.
func RequestID(ctx context.Context) string {
	v, _ := ctx.Value(keyRequestID).(string)
	return v
}

// WithTouch stores the attribution snapshot of the current request.
func WithTouch(ctx context.Context, t analytics.Touch) context.Context {
	return context.WithValue(ctx, keyTouch, t)
}

// Touch returns the attribution snapshot and whether attribution ran.
func Touch(ctx context.Context) (analytics.Touch, bool) {
	v, ok := ctx.Value(keyTouch).(analytics.Touch)
	return v, ok
}

// WithSessionID stores the analytics session id.
func WithSessionID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, keySessionID, id)
}

// SessionID returns the analytics session id.
func SessionID(ctx context.Context) (uuid.UUID, bool) {
	v, ok := ctx.Value(keySessionID).(uuid.UUID)
	return v, ok
}

// WithBot marks the request as coming from a known crawler.
func WithBot(ctx context.Context, isBot bool) context.Context {
	return context.WithValue(ctx, keyBot, isBot)
}

// IsBot reports whether attribution classified the request as a crawler.
func IsBot(ctx context.Context) bool {
	v, _ := ctx.Value(keyBot).(bool)
	return v
}

// WithCSRFToken stores the token the templates must echo back.
func WithCSRFToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, keyCSRFToken, token)
}

// CSRFToken returns the token for the current request.
func CSRFToken(ctx context.Context) string {
	v, _ := ctx.Value(keyCSRFToken).(string)
	return v
}

// WithAdmin stores the authenticated administrator.
func WithAdmin(ctx context.Context, a Admin) context.Context {
	return context.WithValue(ctx, keyAdmin, a)
}

// AdminFrom returns the authenticated administrator, if any.
func AdminFrom(ctx context.Context) (Admin, bool) {
	v, ok := ctx.Value(keyAdmin).(Admin)
	return v, ok
}
