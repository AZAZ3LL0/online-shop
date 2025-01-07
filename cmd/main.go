package main

import (
	"log"
	"my-tshirt-shop/internal/config"
	"my-tshirt-shop/internal/pkg/psql"

	"github.com/gin-gonic/gin"
)

func startApp() {
	r := gin.Default()

	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "Welcome to My T-Shirt Shop!",
		})
	})

	r.Run(":8080")
}

func main() {

	cfg, err := config.NewAppConfig()
	if err != nil {
		panic(err)
	}

	db, err := psql.NewDB(cfg.DBConfig)
	if err != nil {
		log.Fatalf("Error connecting to the database: %v", err)
	}
	defer db.Close()
	log.Println("Successful Connection")

	startApp()
}
