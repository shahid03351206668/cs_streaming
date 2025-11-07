package main

import (
	"log"
	"tasksy/controllers"
	"tasksy/db"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

var PORT string = "8080"

func main() {
	godotenv.Load()

	router := gin.Default()
	db.ConnectDB()
	db.ApplyMigrations()

	router.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})

	router.POST("/api/user/create", controllers.CreateUser)
	router.POST("/api/auth/login", controllers.Login)
	router.GET("/api/user/list", controllers.GetUsers)

	log.Printf("Server starting on port %s", PORT)

	if err := router.Run(":" + PORT); err != nil {
		log.Fatalf("failed to run server: %v", err)
	}

}
