package telegram

import (
	"context"
	"errors"
	"log/slog"
	"sync"
)

// ErrFakeFailure is what the fake returns when told to fail, so retry and
// backoff behaviour can be tested without the Bot API.
var ErrFakeFailure = errors.New("telegram: fake failure")

// SentMessage is one message the fake accepted.
type SentMessage struct {
	ChatID int64
	Text   string
}

// Fake is the in-process bot: it logs messages and keeps them for assertions.
type Fake struct {
	log *slog.Logger

	mu       sync.Mutex
	sent     []SentMessage
	failNext error
	webhook  string
}

// NewFake builds the fake bot.
func NewFake(log *slog.Logger) *Fake { return &Fake{log: log} }

var _ Bot = (*Fake)(nil)

// SendMessage records the message and writes it to the log.
func (f *Fake) SendMessage(ctx context.Context, chatID int64, text string, _ *Keyboard) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failNext != nil {
		err := f.failNext
		f.failNext = nil
		return err
	}
	f.sent = append(f.sent, SentMessage{ChatID: chatID, Text: text})
	if f.log != nil {
		f.log.Info("fake telegram message", slog.Int64("chat_id", chatID), slog.String("text", text))
	}
	return nil
}

// SetWebhook records the webhook URL the application would register.
func (f *Fake) SetWebhook(_ context.Context, url, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.webhook = url
	return nil
}

// FailNext makes the next SendMessage call fail.
func (f *Fake) FailNext(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failNext = err
}

// Sent returns a copy of everything the fake has accepted.
func (f *Fake) Sent() []SentMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]SentMessage, len(f.sent))
	copy(out, f.sent)
	return out
}
