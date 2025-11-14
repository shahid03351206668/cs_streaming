package main

import (
	"log"
	"tasksy/controllers"
	"tasksy/db"
	"tasksy/lib"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

var PORT string = "8080"

func main() {
	godotenv.Load()
	router := gin.Default()

	db.ConnectDB()
	db.ApplyMigrations()

	router.GET("/api/user/list", controllers.GetUsers)
	router.POST("/api/user/register", controllers.RegisterUser)
	router.POST("/api/auth/login", controllers.LoginController)
	router.POST("/api/auth/verify-user", controllers.VerifyUser)
	router.POST("/api/auth/refresh", controllers.RefreshTokenController)
	router.POST("/api/auth/google-auth", controllers.GoogleSignInFirebaseController)

	protected := router.Group("/")
	protected.Use(lib.AuthenticatedHandler)
	{
		protected.GET("api/user/profile", controllers.GetProfile)
		protected.POST("/api/user/update", controllers.UpdateProfile)
		protected.POST("/api/user/change-password", controllers.ChangePassword)
	}

	log.Printf("Server starting on port %s", PORT)
	if err := router.Run(":" + PORT); err != nil {
		log.Fatalf("failed to run server: %v", err)
	}

}
