package telegram

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// InitDataMaxAge is how old a Mini App launch may be, tech.md §5.5.
const InitDataMaxAge = 15 * time.Minute

// ErrInitData marks every rejected Mini App launch. It carries no detail on
// purpose: the caller answers 403 without telling which check failed.
var ErrInitData = errors.New("telegram: init data rejected")

// initDataSecretKey is the constant Telegram derives the signing key from.
const initDataSecretKey = "WebAppData"

// User is a Telegram account: the operator behind a Mini App launch and the
// sender of an incoming bot message are both decoded into this.
type User struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

// Name is what the panel shows for the signed-in operator.
func (u User) Name() string {
	if u.Username != "" {
		return "@" + u.Username
	}
	name := strings.TrimSpace(u.FirstName + " " + u.LastName)
	if name != "" {
		return name
	}
	return strconv.FormatInt(u.ID, 10)
}

// InitData is a verified Mini App launch payload.
type InitData struct {
	User     User
	AuthDate time.Time
	QueryID  string
}

// VerifyInitData checks the launch payload Telegram hands the Mini App and
// returns it only when the signature, the freshness and the user are all sound
// (tech.md §5.5). Every failure is ErrInitData: the caller must not be able to
// tell an expired launch from a forged one.
//
// The algorithm is Telegram's: the key is HMAC_SHA256("WebAppData", bot_token),
// the message is every field except hash and signature, sorted by name and
// joined with newlines.
func VerifyInitData(raw, botToken string, now time.Time, maxAge time.Duration) (InitData, error) {
	if botToken == "" {
		// Without a token there is nothing to verify against, and an empty key
		// would happily validate anything an attacker signs with it.
		return InitData{}, fmt.Errorf("%w: no bot token configured", ErrInitData)
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return InitData{}, fmt.Errorf("%w: malformed payload", ErrInitData)
	}
	// A repeated key would let a forger append a second user= that the check
	// string ignores; the whole payload is refused instead of guessing.
	for _, v := range values {
		if len(v) > 1 {
			return InitData{}, fmt.Errorf("%w: repeated field", ErrInitData)
		}
	}
	got, err := hex.DecodeString(values.Get("hash"))
	if err != nil || len(got) == 0 {
		return InitData{}, fmt.Errorf("%w: no hash", ErrInitData)
	}
	if !hmac.Equal(got, initDataMAC(values, botToken)) {
		return InitData{}, fmt.Errorf("%w: bad signature", ErrInitData)
	}

	seconds, err := strconv.ParseInt(values.Get("auth_date"), 10, 64)
	if err != nil {
		return InitData{}, fmt.Errorf("%w: no auth date", ErrInitData)
	}
	authDate := time.Unix(seconds, 0).UTC()
	// Both directions are rejected: a stale launch is a replay, a future one is
	// a clock the server cannot reason about.
	if now.Sub(authDate) > maxAge || authDate.Sub(now) > maxAge {
		return InitData{}, fmt.Errorf("%w: stale auth date", ErrInitData)
	}

	var user User
	if err := json.Unmarshal([]byte(values.Get("user")), &user); err != nil {
		return InitData{}, fmt.Errorf("%w: no user", ErrInitData)
	}
	if user.ID == 0 {
		return InitData{}, fmt.Errorf("%w: no user id", ErrInitData)
	}
	return InitData{User: user, AuthDate: authDate, QueryID: values.Get("query_id")}, nil
}

// SignInitData produces a payload VerifyInitData accepts. It exists so the
// tests and the local development launcher can exercise the Mini App without
// the Telegram client; it is the same algorithm read backwards.
func SignInitData(values url.Values, botToken string) string {
	signed := url.Values{}
	for k, v := range values {
		if k == "hash" || k == "signature" {
			continue
		}
		signed[k] = v
	}
	signed.Set("hash", hex.EncodeToString(initDataMAC(signed, botToken)))
	return signed.Encode()
}

// initDataMAC builds the data check string and signs it.
func initDataMAC(values url.Values, botToken string) []byte {
	pairs := make([]string, 0, len(values))
	for key := range values {
		// hash is the signature itself; signature is Telegram's separate
		// third-party Ed25519 field and is never part of the check string.
		if key == "hash" || key == "signature" {
			continue
		}
		pairs = append(pairs, key+"="+values.Get(key))
	}
	sort.Strings(pairs)

	secret := hmac.New(sha256.New, []byte(initDataSecretKey))
	secret.Write([]byte(botToken))

	mac := hmac.New(sha256.New, secret.Sum(nil))
	mac.Write([]byte(strings.Join(pairs, "\n")))
	return mac.Sum(nil)
}
