package order

import (
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Field length bounds, counted in runes and not in bytes: every limit in this
// project is a limit on characters, tech.md §16.1.
const (
	MinNameRunes    = 2
	MaxNameRunes    = 100
	MinAddressRunes = 10
	MaxAddressRunes = 300
	MaxContactRunes = 100
	MaxCommentRunes = 500
)

// ErrValidation marks a checkout form the buyer has to fix.
var ErrValidation = errors.New("order: invalid checkout form")

// Contact is either an e-mail or a Telegram @username, tech.md §15.
var (
	reEmail    = regexp.MustCompile(`^[^\s@]+@[^\s@.]+(\.[^\s@.]+)+$`)
	reUsername = regexp.MustCompile(`^@[A-Za-z0-9_]{4,31}$`)
)

// FieldErrors maps a form field name onto the message shown next to it.
type FieldErrors map[string]string

// Any reports whether the form carries at least one error.
func (f FieldErrors) Any() bool { return len(f) > 0 }

// Normalize trims the buyer's input the way it is stored.
func Normalize(c Customer) Customer {
	return Customer{
		Name:    strings.TrimSpace(c.Name),
		Contact: strings.TrimSpace(c.Contact),
		Address: strings.TrimSpace(c.Address),
		Comment: strings.TrimSpace(c.Comment),
	}
}

// Validate checks a normalized customer block. Messages are for the buyer and
// give nothing internal away.
func Validate(c Customer) FieldErrors {
	errs := FieldErrors{}
	if n := utf8.RuneCountInString(c.Name); n < MinNameRunes || n > MaxNameRunes {
		errs["name"] = "Enter your name, up to 100 characters."
	}
	switch {
	case utf8.RuneCountInString(c.Contact) > MaxContactRunes:
		errs["contact"] = "That contact is too long."
	case !validContact(c.Contact):
		errs["contact"] = "Enter an e-mail or a Telegram @username."
	}
	if n := utf8.RuneCountInString(c.Address); n < MinAddressRunes || n > MaxAddressRunes {
		errs["address"] = "Enter the full shipping address, up to 300 characters."
	}
	if utf8.RuneCountInString(c.Comment) > MaxCommentRunes {
		errs["comment"] = "The comment is limited to 500 characters."
	}
	return errs
}

func validContact(v string) bool {
	return reEmail.MatchString(v) || reUsername.MatchString(v)
}
