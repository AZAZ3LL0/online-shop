package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// sendTimeout bounds a single outgoing message, tech.md §16.2.
const sendTimeout = 3 * time.Second

// maxBodyBytes caps how much of an API answer is read into memory.
const maxBodyBytes = 1 << 20

// Client is the real Bot API client.
type Client struct {
	token   string
	baseURL string
	http    *http.Client
}

// NewClient builds the real bot client with a bounded HTTP client.
func NewClient(token string) *Client {
	return &Client{
		token:   token,
		baseURL: "https://api.telegram.org",
		http:    &http.Client{Timeout: sendTimeout},
	}
}

var _ Bot = (*Client)(nil)

// SendMessage delivers one message, optionally with an inline keyboard.
func (c *Client) SendMessage(ctx context.Context, chatID int64, text string, kb *Keyboard) error {
	payload := map[string]any{
		"chat_id": chatID,
		"text":    text,
	}
	if kb != nil {
		payload["reply_markup"] = map[string]any{"inline_keyboard": encodeKeyboard(kb)}
	}
	return c.call(ctx, "sendMessage", payload)
}

// SetWebhook points the bot at our callback URL.
func (c *Client) SetWebhook(ctx context.Context, url, secret string) error {
	return c.call(ctx, "setWebhook", map[string]any{
		"url":          url,
		"secret_token": secret,
	})
}

func (c *Client) call(ctx context.Context, method string, payload map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", method, err)
	}
	endpoint := fmt.Sprintf("%s/bot%s/%s", c.baseURL, c.token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build %s request: %w", method, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("telegram %s: %w", method, err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBodyBytes))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram %s: status %d", method, resp.StatusCode)
	}
	return nil
}

func encodeKeyboard(kb *Keyboard) [][]map[string]string {
	rows := make([][]map[string]string, 0, len(kb.Rows))
	for _, row := range kb.Rows {
		encoded := make([]map[string]string, 0, len(row))
		for _, b := range row {
			encoded = append(encoded, map[string]string{"text": b.Text, "url": b.URL})
		}
		rows = append(rows, encoded)
	}
	return rows
}
