package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/qzq-kiim/shop/internal/repository"
)

type ProductHandler struct {
	productRepo *repository.ProductRepo
}

func NewProductHandler(productRepo *repository.ProductRepo) *ProductHandler {
	return &ProductHandler{productRepo: productRepo}
}

func (h *ProductHandler) Show(c *gin.Context) {
	slug := c.Param("slug")
	product, err := h.productRepo.GetBySlug(c.Request.Context(), slug)
	if err != nil {
		render(c, http.StatusNotFound, "home.html", gin.H{"Error": "Товар не найден"})
		return
	}
	render(c, http.StatusOK, "home.html", gin.H{"Products": nil, "FocusProduct": product})
}

func (h *ProductHandler) Image(c *gin.Context) {
	slug := c.Param("slug")
	side := c.DefaultQuery("side", "front")
	if side != "front" && side != "back" {
		side = "front"
	}

	product, err := h.productRepo.GetBySlug(c.Request.Context(), slug)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	var filename string
	for _, img := range product.Images {
		if img.Side == side {
			filename = img.Filename
			break
		}
	}

	renderPartial(c, http.StatusOK, "product_image.html", gin.H{
		"Filename": filename,
		"Side":     side,
		"Product":  product,
	})
}
