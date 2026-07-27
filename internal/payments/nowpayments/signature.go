package nowpayments

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	"github.com/qzq-kiim/shop/internal/domain/payment"
)

func bytesReader(b []byte) io.Reader { return bytes.NewReader(b) }

// canonicalJSON re-encodes a body with lexicographically sorted keys, which is
// what NOWPayments signs. encoding/json sorts map keys at every level.
func canonicalJSON(rawBody []byte) ([]byte, error) {
	var v map[string]any
	dec := json.NewDecoder(bytes.NewReader(rawBody))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("canonical json: %w", err)
	}
	out, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("canonical json: %w", err)
	}
	return out, nil
}

// Sign returns the hex HMAC-SHA512 of the canonical body. Exported so the dev
// payment page can produce a callback the real verifier accepts.
func Sign(secret, rawBody []byte) (string, error) {
	canonical, err := canonicalJSON(rawBody)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha512.New, secret)
	mac.Write(canonical)
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// verifySignature checks the x-nowpayments-sig header before the body is used
// for anything else.
func verifySignature(secret, rawBody []byte, signature string) error {
	want, err := Sign(secret, rawBody)
	if err != nil {
		return fmt.Errorf("%w: %w", payment.ErrInvalidSignature, err)
	}
	got, err := hex.DecodeString(signature)
	if err != nil {
		return payment.ErrInvalidSignature
	}
	wantRaw, err := hex.DecodeString(want)
	if err != nil {
		return payment.ErrInvalidSignature
	}
	if !hmac.Equal(got, wantRaw) {
		return payment.ErrInvalidSignature
	}
	return nil
}
