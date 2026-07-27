package nowpayments

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// apiBase is the production endpoint, tech.md §5.4.
const apiBase = "https://api.nowpayments.io/v1"

// invoiceTimeout bounds invoice creation, tech.md §16.2.
const invoiceTimeout = 5 * time.Second

// maxBodyBytes caps how much of a provider answer is read into memory.
const maxBodyBytes = 1 << 20

// Client is the real NOWPayments provider.
type Client struct {
	apiKey    string
	ipnSecret []byte
	baseURL   string
	http      *http.Client
}

// NewClient builds the real provider with a bounded HTTP client.
func NewClient(apiKey, ipnSecret string) *Client {
	return &Client{
		apiKey:    apiKey,
		ipnSecret: []byte(ipnSecret),
		baseURL:   apiBase,
		http:      &http.Client{Timeout: invoiceTimeout},
	}
}

var _ Provider = (*Client)(nil)

// CreateInvoice asks the provider for a hosted payment page.
func (c *Client) CreateInvoice(ctx context.Context, in InvoiceRequest) (Invoice, error) {
	ctx, cancel := context.WithTimeout(ctx, invoiceTimeout)
	defer cancel()

	body, err := json.Marshal(newInvoicePayload(in))
	if err != nil {
		return Invoice{}, fmt.Errorf("marshal invoice: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/invoice", bytes.NewReader(body))
	if err != nil {
		return Invoice{}, fmt.Errorf("build invoice request: %w", err)
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return Invoice{}, fmt.Errorf("create invoice: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return Invoice{}, fmt.Errorf("read invoice response: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return Invoice{}, fmt.Errorf("create invoice: provider returned %d", resp.StatusCode)
	}

	var out struct {
		ID         json.Number `json:"id"`
		InvoiceURL string      `json:"invoice_url"`
	}
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		return Invoice{}, fmt.Errorf("decode invoice response: %w", err)
	}
	if out.InvoiceURL == "" {
		return Invoice{}, fmt.Errorf("create invoice: provider returned no invoice_url")
	}
	return Invoice{ID: out.ID.String(), URL: out.InvoiceURL}, nil
}

// VerifySignature checks the IPN HMAC before the body is parsed.
func (c *Client) VerifySignature(rawBody []byte, signature string) error {
	return verifySignature(c.ipnSecret, rawBody, signature)
}

// ParseIPN decodes a verified callback body.
func (c *Client) ParseIPN(rawBody []byte) (IPN, error) { return parseIPN(rawBody) }
