package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type CartHandler struct{}

func NewCartHandler() *CartHandler { return &CartHandler{} }

func (h *CartHandler) Show(c *gin.Context) {
	render(c, http.StatusOK, "cart.html", gin.H{})
}

func (h *CartHandler) Add(c *gin.Context) {
	count := c.PostForm("count")
	if count == "" {
		count = "0"
	}
	renderPartial(c, http.StatusOK, "cart_count.html", gin.H{"Count": count})
}
