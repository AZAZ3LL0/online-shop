package telegram

import (
	"encoding/json"
	"regexp"
	"sort"
)

// themeKeys is the closed set of themeParams the Mini App layout styles itself
// from. Anything Telegram adds later is ignored rather than passed through: the
// values end up in a stylesheet, so the set has to stay known.
var themeKeys = []string{
	"bg_color",
	"secondary_bg_color",
	"section_bg_color",
	"header_bg_color",
	"text_color",
	"hint_color",
	"subtitle_text_color",
	"link_color",
	"accent_text_color",
	"button_color",
	"button_text_color",
	"destructive_text_color",
}

// hexColor is the only value shape accepted. Nothing else may reach the
// stylesheet: a themeParams payload is attacker-controllable input.
var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// ThemeParams is the sanitised Telegram colour scheme, keyed by themeParams
// name without the leading marker.
type ThemeParams map[string]string

// ParseThemeParams reads the tgWebAppThemeParams JSON object and keeps only
// known keys with well-formed hex colours.
func ParseThemeParams(raw string) ThemeParams {
	if raw == "" {
		return nil
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil
	}
	theme := make(ThemeParams, len(themeKeys))
	for _, key := range themeKeys {
		value, ok := decoded[key].(string)
		if ok && hexColor.MatchString(value) {
			theme[key] = value
		}
	}
	if len(theme) == 0 {
		return nil
	}
	return theme
}

// Encode serialises the sanitised theme for the signed cookie it travels in.
func (t ThemeParams) Encode() string {
	if len(t) == 0 {
		return ""
	}
	encoded, err := json.Marshal(map[string]string(t))
	if err != nil {
		return ""
	}
	return string(encoded)
}

// Vars renders the theme as CSS custom property declarations, sorted so the
// same theme always produces the same stylesheet.
func (t ThemeParams) Vars() string {
	if len(t) == 0 {
		return ""
	}
	keys := make([]string, 0, len(t))
	for key := range t {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var out []byte
	for _, key := range keys {
		value := t[key]
		if !hexColor.MatchString(value) {
			continue
		}
		out = append(out, "--tg-"...)
		out = append(out, key...)
		out = append(out, ':')
		out = append(out, value...)
		out = append(out, ';')
	}
	return string(out)
}
