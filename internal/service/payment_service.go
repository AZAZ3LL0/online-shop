package service

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/qzq-kiim/shop/internal/model"
)

type PaymentService struct {
	apiKey     string
	ipnSecret  string
	successURL string
	cancelURL  string
	callbackURL string
}

func NewPaymentService(apiKey, ipnSecret, successURL, cancelURL, baseURL string) *PaymentService {
	return &PaymentService{
		apiKey:      apiKey,
		ipnSecret:   ipnSecret,
		successURL:  successURL,
		cancelURL:   cancelURL,
		callbackURL: baseURL + "/nowpayments/webhook",
	}
}

type invoiceRequest struct {
	PriceAmount    float64 `json:"price_amount"`
	PriceCurrency  string  `json:"price_currency"`
	OrderID        string  `json:"order_id"`
	OrderDesc      string  `json:"order_description"`
	SuccessURL     string  `json:"success_url"`
	CancelURL      string  `json:"cancel_url"`
	IPNCallbackURL string  `json:"ipn_callback_url"`
}

type invoiceResponse struct {
	ID         string `json:"id"`
	PaymentURL string `json:"invoice_url"`
}

func (s *PaymentService) CreateInvoice(order *model.Order) (id, paymentURL string, err error) {
	reqBody := invoiceRequest{
		PriceAmount:    float64(order.TotalAmount) / 100,
		PriceCurrency:  "KZT",
		OrderID:        order.UUID.String(),
		OrderDesc:      buildOrderDesc(order),
		SuccessURL:     s.successURL + order.UUID.String(),
		CancelURL:      s.cancelURL,
		IPNCallbackURL: s.callbackURL,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", "", err
	}

	req, err := http.NewRequest("POST", "https://api.nowpayments.io/v1/invoice", bytes.NewBuffer(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("x-api-key", s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", "", fmt.Errorf("nowpayments returned status %d", resp.StatusCode)
	}

	var inv invoiceResponse
	if err := json.NewDecoder(resp.Body).Decode(&inv); err != nil {
		return "", "", err
	}
	return inv.ID, inv.PaymentURL, nil
}

func (s *PaymentService) VerifyWebhook(body []byte, signature string) bool {
	if s.ipnSecret == "" || signature == "" {
		return false
	}

	// NowPayments signs the JSON payload with keys sorted alphabetically,
	// not the raw request body. Re-marshal through a map (encoding/json
	// sorts map keys, recursively) using json.Number to preserve the
	// original numeric formatting.
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var payload map[string]interface{}
	if err := dec.Decode(&payload); err != nil {
		return false
	}
	sorted, err := json.Marshal(payload)
	if err != nil {
		return false
	}

	mac := hmac.New(sha512.New, []byte(s.ipnSecret))
	mac.Write(sorted)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

func buildOrderDesc(order *model.Order) string {
	if len(order.Items) == 0 {
		return "qzq.kiim order"
	}
	desc := "qzq.kiim"
	for _, item := range order.Items {
		desc += fmt.Sprintf(" · %s %s", item.ProductName, item.Size)
	}
	return desc
}
