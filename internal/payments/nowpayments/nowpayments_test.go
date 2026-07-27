package nowpayments_test

import (
	"context"

	"errors"
	"testing"

	"github.com/qzq-kiim/shop/internal/domain/payment"
	"github.com/qzq-kiim/shop/internal/money"
	"github.com/qzq-kiim/shop/internal/payments/nowpayments"
)

// ipnBody is the payload shape frozen in tech.md §5.4.
const ipnBody = `{"payment_id":5077125051,"payment_status":"finished",` +
	`"pay_address":"0x1","price_amount":35.00,"price_currency":"usd",` +
	`"pay_amount":0.0123,"actually_paid":0.0123,"pay_currency":"eth",` +
	`"order_id":"ORD-260727-0001"}`

func TestFakeInvoiceIsDeterministic(t *testing.T) {
	f := nowpayments.NewFake([]byte("dev-secret"))
	inv, err := f.CreateInvoice(context.Background(), nowpayments.InvoiceRequest{
		Total:       money.New(3500, "USD"),
		OrderNumber: "ORD-260727-0001",
	})
	if err != nil {
		t.Fatalf("create invoice: %v", err)
	}
	if inv.URL != "/dev/pay/ORD-260727-0001" {
		t.Errorf("invoice url = %q", inv.URL)
	}
	if len(f.Invoices()) != 1 {
		t.Errorf("fake recorded %d invoices, want 1", len(f.Invoices()))
	}
}

func TestFakeRejectsGarbage(t *testing.T) {
	f := nowpayments.NewFake([]byte("dev-secret"))
	if _, err := f.CreateInvoice(context.Background(), nowpayments.InvoiceRequest{}); err == nil {
		t.Fatal("empty invoice request must be rejected")
	}
	if _, err := f.CreateInvoice(context.Background(), nowpayments.InvoiceRequest{
		Total:       money.New(0, "USD"),
		OrderNumber: "ORD-1",
	}); err == nil {
		t.Fatal("zero total must be rejected")
	}
}

func TestFakePropagatesFailure(t *testing.T) {
	f := nowpayments.NewFake([]byte("dev-secret"))
	f.FailNext(nowpayments.ErrFakeFailure)
	_, err := f.CreateInvoice(context.Background(), nowpayments.InvoiceRequest{
		Total:       money.New(3500, "USD"),
		OrderNumber: "ORD-1",
	})
	if !errors.Is(err, nowpayments.ErrFakeFailure) {
		t.Fatalf("want ErrFakeFailure, got %v", err)
	}
}

func TestSignatureRoundTrip(t *testing.T) {
	f := nowpayments.NewFake([]byte("dev-secret"))
	sig, err := f.SignBody([]byte(ipnBody))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := f.VerifySignature([]byte(ipnBody), sig); err != nil {
		t.Fatalf("verify own signature: %v", err)
	}
}

// The signature is computed over the body with sorted keys, so a reordered but
// equivalent body must verify with the same signature.
func TestSignatureIsKeyOrderIndependent(t *testing.T) {
	f := nowpayments.NewFake([]byte("dev-secret"))
	sig, err := f.SignBody([]byte(ipnBody))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	// Same fields and same decimal text, different key order on the wire.
	const reordered = `{"order_id":"ORD-260727-0001","actually_paid":0.0123,` +
		`"pay_currency":"eth","payment_status":"finished","pay_amount":0.0123,` +
		`"price_currency":"usd","price_amount":35.00,"pay_address":"0x1",` +
		`"payment_id":5077125051}`
	if err := f.VerifySignature([]byte(reordered), sig); err != nil {
		t.Fatalf("reordered body must verify: %v", err)
	}
}

func TestSignatureRejectsTamperedBody(t *testing.T) {
	f := nowpayments.NewFake([]byte("dev-secret"))
	sig, err := f.SignBody([]byte(ipnBody))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	tampered := `{"payment_id":5077125051,"payment_status":"finished","order_id":"ORD-EVIL"}`
	if err := f.VerifySignature([]byte(tampered), sig); !errors.Is(err, payment.ErrInvalidSignature) {
		t.Fatalf("want ErrInvalidSignature, got %v", err)
	}
	if err := f.VerifySignature([]byte(ipnBody), "not-hex"); !errors.Is(err, payment.ErrInvalidSignature) {
		t.Fatalf("want ErrInvalidSignature for malformed header, got %v", err)
	}
}

func TestParseIPNMatchesContract(t *testing.T) {
	f := nowpayments.NewFake([]byte("dev-secret"))
	ipn, err := f.ParseIPN([]byte(ipnBody))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ipn.PaymentID != "5077125051" {
		t.Errorf("payment id = %q", ipn.PaymentID)
	}
	if ipn.PaymentStatus != payment.ProviderFinished {
		t.Errorf("status = %q", ipn.PaymentStatus)
	}
	if ipn.OrderID != "ORD-260727-0001" {
		t.Errorf("order id = %q", ipn.OrderID)
	}
	// Decimals must survive as text, never as float.
	if ipn.ActuallyPaid != "0.0123" || ipn.PayAmount != "0.0123" {
		t.Errorf("amounts = %q / %q", ipn.PayAmount, ipn.ActuallyPaid)
	}
}

func TestParseIPNRejectsGarbage(t *testing.T) {
	f := nowpayments.NewFake([]byte("dev-secret"))
	if _, err := f.ParseIPN([]byte(`{"foo":"bar"}`)); err == nil {
		t.Fatal("body without payment_id must be rejected")
	}
	if _, err := f.ParseIPN([]byte(`not json`)); err == nil {
		t.Fatal("non-json body must be rejected")
	}
}
