-- name: UpsertPayment :one
INSERT INTO payments (order_id, provider, provider_payment_id, invoice_url,
                      pay_currency, pay_amount, actually_paid, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (provider, provider_payment_id) DO UPDATE
SET status        = EXCLUDED.status,
    invoice_url   = COALESCE(EXCLUDED.invoice_url, payments.invoice_url),
    pay_currency  = COALESCE(EXCLUDED.pay_currency, payments.pay_currency),
    pay_amount    = COALESCE(EXCLUDED.pay_amount, payments.pay_amount),
    actually_paid = COALESCE(EXCLUDED.actually_paid, payments.actually_paid),
    updated_at    = now()
RETURNING id;

-- name: GetPaymentByProviderID :one
SELECT id, order_id, invoice_url, status
FROM payments
WHERE provider = $1 AND provider_payment_id = $2;

-- name: InsertPaymentEvent :execrows
INSERT INTO payment_events (payment_id, provider_status, payload, signature_ok, dedup_key)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (dedup_key) DO NOTHING;
