// Package nowpayments talks to the NOWPayments invoice API and verifies its IPN
// callbacks. The rest of the application depends on Provider, never on the
// concrete client, so the fake is a one-line substitution.
package nowpayments

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/qzq-kiim/shop/internal/money"
)

// InvoiceRequest is what the checkout hands to the provider.
type InvoiceRequest struct {
	Total          money.Amount
	OrderNumber    string
	Description    string
	IPNCallbackURL string
	SuccessURL     string
	CancelURL      string
}

// Invoice is the provider's answer: where to send the buyer.
type Invoice struct {
	ID  string
	URL string
}

// IPN is one parsed callback. Decimal fields stay strings: money never becomes
// a float on the way in.
type IPN struct {
	PaymentID     string
	PaymentStatus string
	OrderID       string
	PayAmount     string
	ActuallyPaid  string
	PayCurrency   string
	PriceAmount   string
	PriceCurrency string
	Raw           []byte
}

// Provider is the payment integration seam, tech.md §5.4.
type Provider interface {
	CreateInvoice(ctx context.Context, in InvoiceRequest) (Invoice, error)
	VerifySignature(rawBody []byte, signature string) error
	ParseIPN(rawBody []byte) (IPN, error)
}

// invoicePayload is the wire shape of POST /v1/invoice, tech.md §5.4.
type invoicePayload struct {
	PriceAmount    json.Number `json:"price_amount"`
	PriceCurrency  string      `json:"price_currency"`
	OrderID        string      `json:"order_id"`
	OrderDesc      string      `json:"order_description"`
	IPNCallbackURL string      `json:"ipn_callback_url"`
	SuccessURL     string      `json:"success_url"`
	CancelURL      string      `json:"cancel_url"`
}

// newInvoicePayload renders the request without ever touching a float: the
// decimal is built from minor units by integer arithmetic.
func newInvoicePayload(in InvoiceRequest) invoicePayload {
	cents := in.Total.Cents
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	amount := fmt.Sprintf("%s%d.%02d", sign, cents/100, cents%100)
	return invoicePayload{
		PriceAmount:    json.Number(amount),
		PriceCurrency:  "usd",
		OrderID:        in.OrderNumber,
		OrderDesc:      in.Description,
		IPNCallbackURL: in.IPNCallbackURL,
		SuccessURL:     in.SuccessURL,
		CancelURL:      in.CancelURL,
	}
}

// parseIPN decodes a callback body into IPN, keeping the raw bytes.
func parseIPN(rawBody []byte) (IPN, error) {
	var body struct {
		PaymentID     json.Number `json:"payment_id"`
		PaymentStatus string      `json:"payment_status"`
		OrderID       string      `json:"order_id"`
		PayAmount     json.Number `json:"pay_amount"`
		ActuallyPaid  json.Number `json:"actually_paid"`
		PayCurrency   string      `json:"pay_currency"`
		PriceAmount   json.Number `json:"price_amount"`
		PriceCurrency string      `json:"price_currency"`
	}
	dec := json.NewDecoder(bytesReader(rawBody))
	dec.UseNumber()
	if err := dec.Decode(&body); err != nil {
		return IPN{}, fmt.Errorf("decode ipn: %w", err)
	}
	if body.PaymentID.String() == "" || body.PaymentStatus == "" {
		return IPN{}, fmt.Errorf("decode ipn: payment_id and payment_status are required")
	}
	return IPN{
		PaymentID:     body.PaymentID.String(),
		PaymentStatus: body.PaymentStatus,
		OrderID:       body.OrderID,
		PayAmount:     body.PayAmount.String(),
		ActuallyPaid:  body.ActuallyPaid.String(),
		PayCurrency:   body.PayCurrency,
		PriceAmount:   body.PriceAmount.String(),
		PriceCurrency: body.PriceCurrency,
		Raw:           rawBody,
	}, nil
}
