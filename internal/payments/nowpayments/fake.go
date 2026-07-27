package nowpayments

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// ErrFakeFailure is what the fake returns when it is told to fail, so the error
// paths of the checkout can be tested without a provider.
var ErrFakeFailure = errors.New("nowpayments: fake failure")

// Fake is the in-process provider used in development and in tests. It signs
// its callbacks with the same algorithm as the real one, so the IPN endpoint is
// exercised for real.
type Fake struct {
	secret []byte

	mu       sync.Mutex
	invoices []InvoiceRequest
	failNext error
}

// NewFake builds the fake provider. The secret signs dev callbacks.
func NewFake(secret []byte) *Fake { return &Fake{secret: secret} }

var _ Provider = (*Fake)(nil)

// CreateInvoice records the request and returns a deterministic local invoice.
func (f *Fake) CreateInvoice(ctx context.Context, in InvoiceRequest) (Invoice, error) {
	if err := ctx.Err(); err != nil {
		return Invoice{}, fmt.Errorf("create invoice: %w", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failNext != nil {
		err := f.failNext
		f.failNext = nil
		return Invoice{}, err
	}
	if in.OrderNumber == "" {
		return Invoice{}, fmt.Errorf("create invoice: order number is required")
	}
	if in.Total.Cents <= 0 {
		return Invoice{}, fmt.Errorf("create invoice: total must be positive")
	}
	f.invoices = append(f.invoices, in)
	return Invoice{
		ID:  "fake-" + in.OrderNumber,
		URL: "/dev/pay/" + in.OrderNumber,
	}, nil
}

// VerifySignature applies the real algorithm with the fake secret.
func (f *Fake) VerifySignature(rawBody []byte, signature string) error {
	return verifySignature(f.secret, rawBody, signature)
}

// ParseIPN decodes a callback body exactly like the real client.
func (f *Fake) ParseIPN(rawBody []byte) (IPN, error) { return parseIPN(rawBody) }

// SignBody produces a header value the verifier accepts, for the dev pay page.
func (f *Fake) SignBody(rawBody []byte) (string, error) { return Sign(f.secret, rawBody) }

// FailNext makes the next CreateInvoice call fail, for error-path tests.
func (f *Fake) FailNext(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failNext = err
}

// Invoices returns a copy of every invoice request the fake has seen.
func (f *Fake) Invoices() []InvoiceRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]InvoiceRequest, len(f.invoices))
	copy(out, f.invoices)
	return out
}
