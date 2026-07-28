package telegram_test

import (
	"bytes"
	"encoding/base64"
	"image/png"
	"strings"
	"testing"

	"github.com/qzq-kiim/shop/internal/telegram"
)

func TestDeepLinkCarriesTheCode(t *testing.T) {
	got := telegram.DeepLink("qzq_shop_bot", "0123456789abcdef")
	want := "https://t.me/qzq_shop_bot?start=0123456789abcdef"
	if got != want {
		t.Errorf("deep link = %q, want %q", got, want)
	}
}

// Without a configured bot there is no link to publish, so callers can hide the
// whole block instead of rendering a broken t.me address.
func TestDeepLinkIsEmptyWithoutABot(t *testing.T) {
	if got := telegram.DeepLink("", "0123456789abcdef"); got != "" {
		t.Errorf("link without a bot username = %q, want empty", got)
	}
	if got := telegram.DeepLink("qzq_shop_bot", ""); got != "" {
		t.Errorf("link without a code = %q, want empty", got)
	}
}

// A username or code that is not URL-safe must not be able to break out of the
// query string and point the button somewhere else.
func TestDeepLinkEscapesItsParts(t *testing.T) {
	got := telegram.DeepLink("bot", "abc&start=evil")
	if strings.Contains(got, "&start=evil") {
		t.Errorf("the code was not escaped: %q", got)
	}
}

// The QR is inlined into the page, so it has to be a real image the browser can
// decode without any further request (tech.md §9.7, no external sources).
func TestQRDataURIDecodesAsPNG(t *testing.T) {
	link := telegram.DeepLink("qzq_shop_bot", "0123456789abcdef")
	uri, err := telegram.QRDataURI(link)
	if err != nil {
		t.Fatalf("encode qr: %v", err)
	}
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(uri, prefix) {
		t.Fatalf("qr uri = %.40q, want a base64 png data uri", uri)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(uri, prefix))
	if err != nil {
		t.Fatalf("decode base64: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	if b := img.Bounds(); b.Dx() == 0 || b.Dy() == 0 {
		t.Errorf("qr image is empty: %v", b)
	}
}

func TestQRDataURIIsEmptyWithoutALink(t *testing.T) {
	uri, err := telegram.QRDataURI("")
	if err != nil {
		t.Fatalf("empty link must not be an error: %v", err)
	}
	if uri != "" {
		t.Errorf("qr for an empty link = %q, want empty", uri)
	}
}
