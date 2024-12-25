package main

import (
	"log"
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

	cfg := &psql.Config{
		Host:     "host",
		Port:     "5432",
		User:     "salavat",
		Password: "salavat",
		DBName:   "shop",
		SSLMode:  "disable",
	}

	db, err := psql.NewDB(cfg)
	if err != nil {
		log.Fatalf("Error connecting to the database: %v", err)
	}
	defer db.Close()
	log.Println("Successful Connection")

	startApp()
}
