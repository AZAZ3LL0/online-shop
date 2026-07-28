package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qzq-kiim/shop/internal/domain/notify"
	"github.com/qzq-kiim/shop/internal/domain/order"
	"github.com/qzq-kiim/shop/internal/storage/postgres/sqlcgen"
	"github.com/qzq-kiim/shop/internal/telegram"
)

// TelegramRepo stores incoming update ids and the chat-to-order links.
type TelegramRepo struct {
	pool *pgxpool.Pool
	q    *sqlcgen.Queries
}

// NewTelegramRepo returns the bot repository bound to the store.
func NewTelegramRepo(s *Store) *TelegramRepo { return &TelegramRepo{pool: s.pool, q: s.q} }

var _ telegram.Repository = (*TelegramRepo)(nil)

// SeenUpdate records an update id and reports whether it was already there. The
// primary key does the deduplication, so two concurrent deliveries of the same
// update cannot both come back false.
func (r *TelegramRepo) SeenUpdate(ctx context.Context, updateID int64) (bool, error) {
	n, err := r.q.InsertTelegramUpdate(ctx, updateID)
	if err != nil {
		return false, fmt.Errorf("insert telegram update: %w", err)
	}
	return n == 0, nil
}

// LinkOrder binds an order to a chat and queues the confirmation in the same
// transaction, so a link never exists without the message that announces it.
func (r *TelegramRepo) LinkOrder(ctx context.Context, code string, chatID int64, username string) (telegram.LinkedOrder, error) {
	var linked telegram.LinkedOrder
	err := withTx(ctx, r.pool, func(q *sqlcgen.Queries) error {
		found, err := q.GetOrderByLinkCode(ctx, code)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return telegram.ErrCodeUnknown
			}
			return fmt.Errorf("get order by link code: %w", err)
		}
		linked = telegram.LinkedOrder{
			ID:     found.ID,
			Number: found.Number,
			Status: order.Status(found.Status),
		}

		_, err = q.InsertTelegramLink(ctx, sqlcgen.InsertTelegramLinkParams{
			OrderID:  found.ID,
			ChatID:   chatID,
			Username: nullString(username),
		})
		if err != nil {
			return fmt.Errorf("insert telegram link: %w", err)
		}

		// A repeated /start finds the link already there and the dedup key
		// already taken, so it produces neither a second row nor a second
		// message.
		return enqueueOrderNotification(ctx, q, linked.ID, notify.KindOrderLinked, linked.Status,
			notify.LinkedText(linked.Number, linked.Status))
	})
	if err != nil {
		return telegram.LinkedOrder{}, err
	}
	return linked, nil
}

// OrdersByChat lists every order a chat follows, newest first.
func (r *TelegramRepo) OrdersByChat(ctx context.Context, chatID int64) ([]telegram.LinkedOrder, error) {
	rows, err := r.q.ListOrdersByChat(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("list orders by chat: %w", err)
	}
	out := make([]telegram.LinkedOrder, 0, len(rows))
	for _, row := range rows {
		out = append(out, telegram.LinkedOrder{
			ID:     row.ID,
			Number: row.Number,
			Status: order.Status(row.Status),
		})
	}
	return out, nil
}
