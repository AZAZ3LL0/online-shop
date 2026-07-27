package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/qzq-kiim/shop/internal/model"
	"github.com/qzq-kiim/shop/internal/repository"
)

type OrderService struct {
	orderRepo   *repository.OrderRepo
	productRepo *repository.ProductRepo
	payment     *PaymentService
}

func NewOrderService(
	orderRepo *repository.OrderRepo,
	productRepo *repository.ProductRepo,
	payment *PaymentService,
) *OrderService {
	return &OrderService{orderRepo: orderRepo, productRepo: productRepo, payment: payment}
}

// maxItemQuantity caps per-line quantity to reject absurd/abusive orders
// even when nominal stock would allow them.
const maxItemQuantity = 100

type CreateOrderRequest struct {
	Name      string
	Email     string
	Phone     string
	City      string
	Street    string
	CartJSON  string
	SessionID string
}

// Create saves the order and returns its UUID. Payment is initiated separately.
func (s *OrderService) Create(ctx context.Context, req CreateOrderRequest) (uuid.UUID, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.TrimSpace(req.Email)
	req.Phone = strings.TrimSpace(req.Phone)
	req.City = strings.TrimSpace(req.City)
	req.Street = strings.TrimSpace(req.Street)
	if req.Name == "" || req.Email == "" || req.Phone == "" || req.City == "" || req.Street == "" {
		return uuid.UUID{}, errors.New("заполните все поля")
	}

	var cart model.Cart
	if err := json.Unmarshal([]byte(req.CartJSON), &cart); err != nil {
		return uuid.UUID{}, errors.New("invalid cart data")
	}
	if len(cart.Items) == 0 {
		return uuid.UUID{}, errors.New("cart is empty")
	}

	// serverTotal is computed exclusively from server-side product prices.
	// The client-supplied cart prices (ci.Price / cart.Total()) are NEVER
	// trusted for money — otherwise a buyer could tamper with localStorage
	// and pay an arbitrary amount.
	serverTotal := 0
	var orderItems []model.OrderItem
	for _, ci := range cart.Items {
		product, err := s.productRepo.GetBySlug(ctx, ci.ProductSlug)
		if err != nil {
			return uuid.UUID{}, fmt.Errorf("product not found: %s", ci.ProductSlug)
		}

		var variant *model.ProductVariant
		for _, v := range product.Variants {
			if v.Size == ci.Size {
				vCopy := v
				variant = &vCopy
				break
			}
		}
		if variant == nil {
			return uuid.UUID{}, fmt.Errorf("size %s not available for %s", ci.Size, ci.ProductSlug)
		}
		if ci.Quantity < 1 || ci.Quantity > maxItemQuantity {
			return uuid.UUID{}, fmt.Errorf("invalid quantity for %s", ci.ProductSlug)
		}
		if variant.Stock < ci.Quantity {
			return uuid.UUID{}, fmt.Errorf("not enough stock for %s size %s", ci.ProductSlug, ci.Size)
		}

		serverTotal += product.Price * ci.Quantity

		orderItems = append(orderItems, model.OrderItem{
			ProductID:   product.ID,
			VariantID:   variant.ID,
			Size:        ci.Size,
			Quantity:    ci.Quantity,
			Price:       product.Price, // authoritative, server-side price
			ProductName: product.Name,
			ProductSlug: product.Slug,
		})
	}

	order := &model.Order{
		SessionID:       req.SessionID,
		Status:          model.StatusPending,
		TotalAmount:     serverTotal,
		Currency:        "KZT",
		CustomerName:    req.Name,
		CustomerEmail:   req.Email,
		CustomerPhone:   req.Phone,
		DeliveryAddress: req.City + ", " + req.Street,
		Items:           orderItems,
	}

	if err := s.orderRepo.Create(ctx, order); err != nil {
		return uuid.UUID{}, fmt.Errorf("create order: %w", err)
	}
	if err := s.orderRepo.AddItems(ctx, order.ID, orderItems); err != nil {
		return uuid.UUID{}, fmt.Errorf("add items: %w", err)
	}

	return order.UUID, nil
}

// CreatePaymentURL creates a NowPayments invoice for an existing pending order.
func (s *OrderService) CreatePaymentURL(ctx context.Context, orderUUID uuid.UUID) (string, error) {
	order, err := s.orderRepo.GetByUUID(ctx, orderUUID)
	if err != nil {
		return "", fmt.Errorf("order not found: %w", err)
	}
	if order.IsPaid() || order.IsCancelled() {
		return "", errors.New("order cannot be paid")
	}

	invID, paymentURL, err := s.payment.CreateInvoice(order)
	if err != nil {
		return "", fmt.Errorf("create invoice: %w", err)
	}

	if err := s.orderRepo.SetPaymentID(ctx, order.ID, invID); err != nil {
		return "", fmt.Errorf("set payment id: %w", err)
	}

	return paymentURL, nil
}
