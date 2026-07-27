package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/qzq-kiim/shop/internal/repository"
)

type HomeHandler struct {
	productRepo *repository.ProductRepo
}

func NewHomeHandler(productRepo *repository.ProductRepo) *HomeHandler {
	return &HomeHandler{productRepo: productRepo}
}

func (h *HomeHandler) Index(c *gin.Context) {
	products, err := h.productRepo.GetAll(c.Request.Context())
	if err != nil {
		render(c, http.StatusInternalServerError, "home.html", gin.H{"Error": err.Error()})
		return
	}
	render(c, http.StatusOK, "home.html", gin.H{"Products": products})
}
