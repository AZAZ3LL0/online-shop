package telegram

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/qzq-kiim/shop/internal/domain/order"
)

// ErrCodeUnknown is returned when no order carries the offered link code. The
// bot answers it exactly like a bare /start: whether an order exists is never
// confirmed to someone guessing codes (tech.md §5.5).
var ErrCodeUnknown = errors.New("telegram: unknown link code")

// LinkedOrder is the little of an order the bot ever needs to talk about. The
// buyer's name, address and contact are deliberately not carried here: a chat
// that guessed its way in must not be able to read them (tech.md §9.11).
type LinkedOrder struct {
	ID     uuid.UUID
	Number string
	Status order.Status
}

// ChatLink is one chat that follows an order, as the admin card lists it.
type ChatLink struct {
	ChatID   int64
	Username string
	LinkedAt time.Time
}

// Repository is the storage the bot depends on, declared here by its consumer
// (tech.md §16.4).
type Repository interface {
	// SeenUpdate records an update id and reports whether it was already on
	// file, which is the whole deduplication rule of tech.md §5.5.
	SeenUpdate(ctx context.Context, updateID int64) (bool, error)
	// LinkOrder binds the order behind code to a chat and queues the
	// confirmation in the same transaction. It returns ErrCodeUnknown when no
	// order carries the code.
	LinkOrder(ctx context.Context, code string, chatID int64, username string) (LinkedOrder, error)
	// OrdersByChat lists every order this chat is following.
	OrdersByChat(ctx context.Context, chatID int64) ([]LinkedOrder, error)
}
