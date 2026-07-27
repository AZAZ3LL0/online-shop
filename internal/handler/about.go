package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func AboutHandler(c *gin.Context) {
	render(c, http.StatusOK, "about.html", gin.H{})
}
