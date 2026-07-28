package telegram

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/qzq-kiim/shop/internal/domain/notify"
)

// The commands the bot answers, tech.md §5.5.
const (
	commandStart  = "/start"
	commandStatus = "/status"
)

// Replies that do not depend on any order. The answer to an unknown code is the
// same as the answer to a bare /start on purpose: a guesser learns nothing.
const (
	helpText = "Send me the link from your order page and I will keep you posted on it. " +
		"Use /status to see every order you are following."
	noOrdersText = "You are not following any order yet. Open the order page in the shop and press " +
		"\"Track order in Telegram\"."
)

// Router turns one incoming update into the bot's answer. It knows nothing
// about HTTP: the webhook hands it a parsed update and nothing else.
type Router struct {
	repo Repository
	bot  Bot
}

// NewRouter wires the bot command router.
func NewRouter(repo Repository, bot Bot) *Router {
	return &Router{repo: repo, bot: bot}
}

// Handle answers one update. Replies are sent straight away; only the linking
// confirmation goes through the outbox, because it is a notification about an
// order and has to survive a Telegram outage (tech.md §5.5).
func (r *Router) Handle(ctx context.Context, u Update) error {
	if u.Message == nil {
		return nil
	}
	msg := *u.Message
	command, argument := msg.Command()

	switch {
	case command == commandStart && argument != "":
		return r.link(ctx, msg, argument)
	case command == commandStart:
		return r.reply(ctx, msg, helpText)
	case command == commandStatus:
		return r.status(ctx, msg)
	default:
		return r.reply(ctx, msg, helpText)
	}
}

// link binds the order behind the deep link code to this chat. The confirmation
// is queued by the repository inside the same transaction as the link itself.
func (r *Router) link(ctx context.Context, msg Message, code string) error {
	_, err := r.repo.LinkOrder(ctx, code, msg.Chat.ID, msg.Username())
	switch {
	case errors.Is(err, ErrCodeUnknown):
		return r.reply(ctx, msg, helpText)
	case err != nil:
		return fmt.Errorf("link order: %w", err)
	}
	// The confirmation is already queued by LinkOrder, so nothing is sent from
	// here and a repeated /start cannot produce a second message.
	return nil
}

// status lists every order this chat follows.
func (r *Router) status(ctx context.Context, msg Message) error {
	orders, err := r.repo.OrdersByChat(ctx, msg.Chat.ID)
	if err != nil {
		return fmt.Errorf("list orders of chat: %w", err)
	}
	if len(orders) == 0 {
		return r.reply(ctx, msg, noOrdersText)
	}
	lines := make([]string, 0, len(orders))
	for _, o := range orders {
		lines = append(lines, notify.StatusText(o.Number, o.Status))
	}
	return r.reply(ctx, msg, strings.Join(lines, "\n"))
}

func (r *Router) reply(ctx context.Context, msg Message, text string) error {
	if err := r.bot.SendMessage(ctx, msg.Chat.ID, text, nil); err != nil {
		return fmt.Errorf("reply to chat %d: %w", msg.Chat.ID, err)
	}
	return nil
}
