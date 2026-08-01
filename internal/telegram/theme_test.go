package telegram_test

import (
	"strings"
	"testing"

	"github.com/qzq-kiim/shop/internal/telegram"
)

// TestParseThemeParamsKeepsOnlyKnownColours is the guard on the one path where
// Telegram-supplied text reaches a stylesheet.
func TestParseThemeParamsKeepsOnlyKnownColours(t *testing.T) {
	theme := telegram.ParseThemeParams(`{
		"bg_color": "#17212b",
		"text_color": "#FFFFFF",
		"hint_color": "red",
		"link_color": "#62bcf9; } body { display:none",
		"unknown_key": "#000000"
	}`)

	if got := theme["bg_color"]; got != "#17212b" {
		t.Errorf("bg_color = %q, want #17212b", got)
	}
	if got := theme["text_color"]; got != "#FFFFFF" {
		t.Errorf("text_color = %q, want #FFFFFF", got)
	}
	for _, key := range []string{"hint_color", "link_color", "unknown_key"} {
		if _, ok := theme[key]; ok {
			t.Errorf("%s survived sanitising: %q", key, theme[key])
		}
	}

	vars := theme.Vars()
	if !strings.Contains(vars, "--tg-bg_color:#17212b;") {
		t.Errorf("css vars do not declare the background: %q", vars)
	}
	if strings.ContainsAny(vars, "}{") {
		t.Errorf("css vars may not carry braces: %q", vars)
	}
}

// TestParseThemeParamsRejectsGarbage keeps a broken payload from producing an
// empty stylesheet block instead of no block at all.
func TestParseThemeParamsRejectsGarbage(t *testing.T) {
	for _, raw := range []string{"", "not json", "[]", `{"bg_color":123}`, `{"hint_color":"#fff"}`} {
		if theme := telegram.ParseThemeParams(raw); theme != nil {
			t.Errorf("ParseThemeParams(%q) = %v, want nil", raw, theme)
		}
	}
}

// TestThemeParamsRoundTrip is what the signed cookie relies on.
func TestThemeParamsRoundTrip(t *testing.T) {
	theme := telegram.ParseThemeParams(`{"bg_color":"#17212b","button_color":"#5288c1"}`)
	restored := telegram.ParseThemeParams(theme.Encode())
	if restored.Vars() != theme.Vars() {
		t.Fatalf("round trip changed the theme: %q vs %q", restored.Vars(), theme.Vars())
	}
}
