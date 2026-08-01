package telegram_test

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/qzq-kiim/shop/internal/telegram"
)

// botToken is a throwaway value in the shape Telegram issues. It signs nothing
// outside this file.
const botToken = "1234567:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw"

func launch(authDate time.Time, user string) url.Values {
	return url.Values{
		"auth_date": {strconv.FormatInt(authDate.Unix(), 10)},
		"query_id":  {"AAHdqTcvAAAAAN2pNy_test"},
		"user":      {user},
	}
}

const operator = `{"id":4242,"username":"az","first_name":"Samat"}`

// TestVerifyInitDataAcceptsAFreshLaunch is the happy path of tech.md §5.5.
func TestVerifyInitDataAcceptsAFreshLaunch(t *testing.T) {
	now := time.Now().UTC()
	raw := telegram.SignInitData(launch(now, operator), botToken)

	data, err := telegram.VerifyInitData(raw, botToken, now, telegram.InitDataMaxAge)
	if err != nil {
		t.Fatalf("verify a freshly signed launch: %v", err)
	}
	if data.User.ID != 4242 {
		t.Errorf("user id = %d, want 4242", data.User.ID)
	}
	if data.User.Name() != "@az" {
		t.Errorf("display name = %q, want @az", data.User.Name())
	}
	if !data.AuthDate.Equal(time.Unix(now.Unix(), 0).UTC()) {
		t.Errorf("auth date = %s, want %s", data.AuthDate, now)
	}
}

// TestVerifyInitDataRejects covers every way a launch is refused. All of them
// have to come back as ErrInitData so the handler cannot leak which one it was.
func TestVerifyInitDataRejects(t *testing.T) {
	now := time.Now().UTC()
	valid := telegram.SignInitData(launch(now, operator), botToken)

	cases := []struct {
		name  string
		raw   string
		token string
		now   time.Time
	}{
		{
			name:  "auth_date older than the window",
			raw:   telegram.SignInitData(launch(now.Add(-16*time.Minute), operator), botToken),
			token: botToken,
			now:   now,
		},
		{
			name:  "auth_date in the future",
			raw:   telegram.SignInitData(launch(now.Add(16*time.Minute), operator), botToken),
			token: botToken,
			now:   now,
		},
		{
			name:  "signed with another bot token",
			raw:   telegram.SignInitData(launch(now, operator), botToken+"x"),
			token: botToken,
			now:   now,
		},
		{
			name:  "hash tampered with",
			raw:   strings.Replace(valid, "hash=", "hash=00", 1),
			token: botToken,
			now:   now,
		},
		{
			name:  "field appended after signing",
			raw:   valid + "&user=" + url.QueryEscape(`{"id":1,"username":"mallory"}`),
			token: botToken,
			now:   now,
		},
		{
			name:  "no hash at all",
			raw:   launch(now, operator).Encode(),
			token: botToken,
			now:   now,
		},
		{
			name:  "no user",
			raw:   telegram.SignInitData(url.Values{"auth_date": {strconv.FormatInt(now.Unix(), 10)}}, botToken),
			token: botToken,
			now:   now,
		},
		{
			name:  "user without an id",
			raw:   telegram.SignInitData(launch(now, `{"username":"az"}`), botToken),
			token: botToken,
			now:   now,
		},
		{
			name:  "no bot token configured",
			raw:   valid,
			token: "",
			now:   now,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := telegram.VerifyInitData(tc.raw, tc.token, tc.now, telegram.InitDataMaxAge); !errors.Is(err, telegram.ErrInitData) {
				t.Fatalf("verify = %v, want ErrInitData", err)
			}
		})
	}
}

// TestVerifyInitDataIgnoresTheSignatureField keeps the check string aligned
// with Telegram: the Ed25519 signature field is never part of it, so a launch
// carrying one still verifies.
func TestVerifyInitDataIgnoresTheSignatureField(t *testing.T) {
	now := time.Now().UTC()
	values := launch(now, operator)
	raw := telegram.SignInitData(values, botToken) + "&signature=abc123"

	if _, err := telegram.VerifyInitData(raw, botToken, now, telegram.InitDataMaxAge); err != nil {
		t.Fatalf("verify a launch carrying a signature field: %v", err)
	}
}
