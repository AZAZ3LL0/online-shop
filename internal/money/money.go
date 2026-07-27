// Package money holds monetary values in minor units so that no arithmetic in
// the project ever touches a float.
package money

import (
	"errors"
	"fmt"
	"strconv"
)

// ErrCurrencyMismatch is returned when two amounts of different currencies meet.
var ErrCurrencyMismatch = errors.New("money: currency mismatch")

// Amount holds money in minor units (cents), without float.
type Amount struct {
	Cents    int64
	Currency string // ISO 4217, "USD"
}

// New builds an Amount from minor units and an ISO 4217 code.
func New(cents int64, currency string) Amount {
	return Amount{Cents: cents, Currency: currency}
}

// Zero returns the zero amount in the given currency.
func Zero(currency string) Amount {
	return Amount{Cents: 0, Currency: currency}
}

// Add returns a+b, failing when the currencies differ.
func (a Amount) Add(b Amount) (Amount, error) {
	if err := a.sameCurrency(b); err != nil {
		return Amount{}, err
	}
	return Amount{Cents: a.Cents + b.Cents, Currency: a.currency()}, nil
}

// Sub returns a-b, failing when the currencies differ.
func (a Amount) Sub(b Amount) (Amount, error) {
	if err := a.sameCurrency(b); err != nil {
		return Amount{}, err
	}
	return Amount{Cents: a.Cents - b.Cents, Currency: a.currency()}, nil
}

// Mul scales the amount by a whole factor, used for line totals (unit price x qty).
func (a Amount) Mul(n int64) Amount {
	return Amount{Cents: a.Cents * n, Currency: a.Currency}
}

// IsZero reports whether the amount carries no value.
func (a Amount) IsZero() bool { return a.Cents == 0 }

// String renders the amount for humans: "$12.00" for USD, "12.00 EUR" otherwise.
func (a Amount) String() string {
	cur := a.currency()
	sign := ""
	cents := a.Cents
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	whole := cents / 100
	frac := cents % 100
	body := sign + strconv.FormatInt(whole, 10) + "." + fmt.Sprintf("%02d", frac)
	if cur == "USD" {
		return "$" + body
	}
	return body + " " + cur
}

// currency defaults an empty code to USD so a zero Amount stays additive.
func (a Amount) currency() string {
	if a.Currency == "" {
		return "USD"
	}
	return a.Currency
}

func (a Amount) sameCurrency(b Amount) error {
	if a.currency() != b.currency() {
		return fmt.Errorf("%w: %s vs %s", ErrCurrencyMismatch, a.currency(), b.currency())
	}
	return nil
}
