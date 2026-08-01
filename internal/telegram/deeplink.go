package telegram

import (
	"encoding/base64"
	"fmt"
	"net/url"

	"rsc.io/qr"
)

// qrLevel trades a little density for the ability to scan a phone screen off a
// desktop monitor, which is exactly what this code is for.
const qrLevel = qr.M

// DeepLink is the bot entry point behind the "Track order in Telegram" button.
// An unconfigured bot username yields an empty link so a caller can hide the
// whole block instead of publishing a broken t.me address.
func DeepLink(botUsername, code string) string {
	if botUsername == "" || code == "" {
		return ""
	}
	return "https://t.me/" + url.PathEscape(botUsername) + "?start=" + url.QueryEscape(code)
}

// QRDataURI renders link as a PNG data URI. The image is inlined rather than
// served from a route: it is derived from the order token and must not become a
// separately guessable address (tech.md §9.10).
func QRDataURI(link string) (string, error) {
	if link == "" {
		return "", nil
	}
	code, err := qr.Encode(link, qrLevel)
	if err != nil {
		return "", fmt.Errorf("telegram: encode qr: %w", err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(code.PNG()), nil
}
