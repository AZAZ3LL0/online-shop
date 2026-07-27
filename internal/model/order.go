package model

import (
	"time"

	"github.com/google/uuid"
)

type OrderStatus string

const (
	StatusPending         OrderStatus = "pending"
	StatusAwaitingPayment OrderStatus = "awaiting_payment"
	StatusPaid            OrderStatus = "paid"
	StatusCancelled       OrderStatus = "cancelled"
)

type Order struct {
	ID              int         `db:"id"`
	UUID            uuid.UUID   `db:"uuid"`
	SessionID       string      `db:"session_id"`
	Status          OrderStatus `db:"status"`
	TotalAmount     int         `db:"total_amount"`
	Currency        string      `db:"currency"`
	CustomerName    string      `db:"customer_name"`
	CustomerEmail   string      `db:"customer_email"`
	CustomerPhone   string      `db:"customer_phone"`
	DeliveryAddress string      `db:"delivery_address"`
	NowpaymentsID   string      `db:"nowpayments_id"`
	CreatedAt       time.Time   `db:"created_at"`
	UpdatedAt       time.Time   `db:"updated_at"`

	Items []OrderItem
}

func (o *Order) FormattedTotal() string {
	tenge := o.TotalAmount / 100
	return formatInt(tenge) + " ₸"
}

func (o *Order) IsPaid() bool      { return o.Status == StatusPaid }
func (o *Order) IsPending() bool   { return o.Status == StatusPending }
func (o *Order) IsAwaiting() bool  { return o.Status == StatusAwaitingPayment }
func (o *Order) IsCancelled() bool { return o.Status == StatusCancelled }

type OrderItem struct {
	ID        int    `db:"id"`
	OrderID   int    `db:"order_id"`
	ProductID int    `db:"product_id"`
	VariantID int    `db:"variant_id"`
	Size      string `db:"size"`
	Quantity  int    `db:"quantity"`
	Price     int    `db:"price"`

	ProductName string
	ProductSlug string
}

func (i *OrderItem) FormattedPrice() string {
	return formatInt(i.Price/100) + " ₸"
}

func (i *OrderItem) Total() int { return i.Price * i.Quantity }

func (i *OrderItem) FormattedTotal() string {
	return formatInt(i.Total()/100) + " ₸"
}

// CartItem — структура из localStorage
type CartItem struct {
	ProductID   int    `json:"product_id"`
	ProductSlug string `json:"product_slug"`
	ProductName string `json:"product_name"`
	Size        string `json:"size"`
	Quantity    int    `json:"quantity"`
	Price       int    `json:"price"`
}

type Cart struct {
	Items []CartItem `json:"items"`
}

func (c *Cart) Total() int {
	sum := 0
	for _, item := range c.Items {
		sum += item.Price * item.Quantity
	}
	return sum
}

func (c *Cart) FormattedTotal() string {
	return formatInt(c.Total()/100) + " ₸"
}

func (c *Cart) Count() int {
	n := 0
	for _, item := range c.Items {
		n += item.Quantity
	}
	return n
}
