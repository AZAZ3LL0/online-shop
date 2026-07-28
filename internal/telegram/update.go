package telegram

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Update is the slice of a Bot API update this shop acts on, tech.md §5.5.
// Everything else Telegram sends is deliberately not decoded.
type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message"`
}

// Message is one incoming chat message.
type Message struct {
	Chat Chat   `json:"chat"`
	From *User  `json:"from"`
	Text string `json:"text"`
}

// Chat is where a message came from; its id is what outgoing messages address.
type Chat struct {
	ID int64 `json:"id"`
}

// User is the sender, kept only for the username stored alongside a link.
type User struct {
	Username string `json:"username"`
}

// ParseUpdate decodes one webhook body. Unknown fields are ignored: the Bot API
// grows, and a new field must never turn into a rejected update.
func ParseUpdate(body []byte) (Update, error) {
	var u Update
	if err := json.Unmarshal(body, &u); err != nil {
		return Update{}, fmt.Errorf("telegram: parse update: %w", err)
	}
	if u.UpdateID == 0 {
		return Update{}, fmt.Errorf("telegram: update carries no update_id")
	}
	return u, nil
}

// Command splits a message into its command and the single argument the deep
// link carries. "/start@qzq_bot code" and "/start code" parse the same way.
func (m Message) Command() (string, string) {
	fields := strings.Fields(m.Text)
	if len(fields) == 0 || !strings.HasPrefix(fields[0], "/") {
		return "", ""
	}
	command, _, _ := strings.Cut(fields[0], "@")
	if len(fields) == 1 {
		return command, ""
	}
	return command, fields[1]
}

// Username is the sender's @name, empty when Telegram withheld it.
func (m Message) Username() string {
	if m.From == nil {
		return ""
	}
	return m.From.Username
}
