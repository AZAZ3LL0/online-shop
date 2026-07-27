package handler

import (
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/qzq-kiim/shop/internal/repository"
	"github.com/qzq-kiim/shop/internal/service"
)

type OrderHandler struct {
	orderRepo    *repository.OrderRepo
	orderService *service.OrderService
}

func NewOrderHandler(orderRepo *repository.OrderRepo, orderService *service.OrderService) *OrderHandler {
	return &OrderHandler{orderRepo: orderRepo, orderService: orderService}
}

func (h *OrderHandler) Create(c *gin.Context) {
	sessionID, _ := c.Get("session_id")

	req := service.CreateOrderRequest{
		Name:      c.PostForm("name"),
		Email:     c.PostForm("email"),
		Phone:     c.PostForm("phone"),
		City:      c.PostForm("city"),
		Street:    c.PostForm("street"),
		CartJSON:  c.PostForm("cart_json"),
		SessionID: sessionID.(string),
	}

	orderUUID, err := h.orderService.Create(c.Request.Context(), req)
	if err != nil {
		render(c, http.StatusBadRequest, "cart.html", gin.H{"Error": err.Error()})
		return
	}

	c.Redirect(http.StatusSeeOther, "/order/"+orderUUID.String())
}

// Pay creates a NowPayments invoice for an existing order and redirects back
// to the order status page with the payment URL as a query param.
func (h *OrderHandler) Pay(c *gin.Context) {
	id, err := uuid.Parse(c.Param("uuid"))
	if err != nil {
		render(c, http.StatusBadRequest, "order.html", gin.H{"Error": "Неверный ID заказа"})
		return
	}

	if !h.ownsOrder(c, id) {
		render(c, http.StatusNotFound, "order.html", gin.H{"Error": "Заказ не найден"})
		return
	}

	paymentURL, err := h.orderService.CreatePaymentURL(c.Request.Context(), id)
	if err != nil {
		order, _ := h.orderRepo.GetByUUID(c.Request.Context(), id)
		render(c, http.StatusBadRequest, "order.html", gin.H{
			"Order": order,
			"Error": "Не удалось создать платёж: " + err.Error(),
		})
		return
	}

	params := url.Values{}
	params.Set("pay_url", paymentURL)
	c.Redirect(http.StatusSeeOther, "/order/"+id.String()+"?"+params.Encode())
}

func (h *OrderHandler) Status(c *gin.Context) {
	id, err := uuid.Parse(c.Param("uuid"))
	if err != nil {
		render(c, http.StatusBadRequest, "order.html", gin.H{"Error": "Неверный ID заказа"})
		return
	}

	order, err := h.orderRepo.GetByUUID(c.Request.Context(), id)
	if err != nil {
		render(c, http.StatusNotFound, "order.html", gin.H{"Error": "Заказ не найден"})
		return
	}

	// Ownership check: an order carries the customer's name, contacts and
	// address. Only the session that created it may view it. A mismatch is a
	// 404 (not 403) so we don't confirm the order's existence to strangers.
	if sid, _ := c.Get("session_id"); order.SessionID != sid {
		render(c, http.StatusNotFound, "order.html", gin.H{"Error": "Заказ не найден"})
		return
	}

	render(c, http.StatusOK, "order.html", gin.H{
		"Order":      order,
		"PaymentURL": c.Query("pay_url"),
	})
}

// ownsOrder reports whether the current session owns the given order.
func (h *OrderHandler) ownsOrder(c *gin.Context, id uuid.UUID) bool {
	order, err := h.orderRepo.GetByUUID(c.Request.Context(), id)
	if err != nil {
		return false
	}
	sid, _ := c.Get("session_id")
	return order.SessionID == sid
}
