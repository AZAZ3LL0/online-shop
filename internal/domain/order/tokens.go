package order

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// Token sizes, tech.md §4 and §9.10: every token is crypto/rand and at least
// 128 bits wide.
const (
	publicTokenBytes = 16 // 32 hex chars
	linkCodeBytes    = 8  // 16 hex chars
	numberSuffixLen  = 4
)

// numberAlphabet avoids the characters that are read back wrong over the phone.
const numberAlphabet = "0123456789ABCDEFGHJKMNPQRSTUVWXYZ"

// NewPublicToken returns the 32 hex chars addressing the order status page.
func NewPublicToken() (string, error) { return randomHex(publicTokenBytes) }

// NewLinkCode returns the 16 hex chars of a Telegram deep link.
func NewLinkCode() (string, error) { return randomHex(linkCodeBytes) }

// NewNumber renders a human-readable order number, ORD-YYMMDD-XXXX.
func NewNumber(now time.Time) (string, error) {
	suffix, err := randomSuffix(numberSuffixLen)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("ORD-%s-%s", now.UTC().Format("060102"), suffix), nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("order: random token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// randomSuffix draws from numberAlphabet without modulo bias: the alphabet is
// read one byte at a time and out-of-range draws are discarded.
func randomSuffix(n int) (string, error) {
	var out strings.Builder
	buf := make([]byte, 1)
	limit := byte(len(numberAlphabet) * (256 / len(numberAlphabet)))
	for out.Len() < n {
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("order: random number: %w", err)
		}
		if buf[0] >= limit {
			continue
		}
		out.WriteByte(numberAlphabet[int(buf[0])%len(numberAlphabet)])
	}
	return out.String(), nil
}
