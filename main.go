package main

import (
	"log"
	"net/http"
	"tasksy/controllers"
	"tasksy/db"
	"tasksy/lib"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

var PORT string = "8080"

func main() {
	godotenv.Load()
	router := gin.Default()
	router.Use(cors.Default())
	router.Static("/media", "./media")

	db.ConnectDB()
	db.ApplyMigrations()

	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusAccepted, gin.H{
			"message": "pong",
		})
	})

	router.GET("/api/user/list", controllers.GetUsers)
	router.POST("/api/user/register", controllers.RegisterUser)
	router.POST("/api/category/create", controllers.CreateCategory)
	router.GET("/api/category/list", controllers.GetCategories)

	router.POST("/api/auth/login", controllers.LoginController)
	router.POST("/api/auth/verify-user", controllers.VerifyUser)
	router.POST("/api/auth/refresh", controllers.RefreshTokenController)
	router.POST("/api/auth/google-auth", controllers.GoogleSignInFirebaseController)
	router.GET("/api/jobs/list", controllers.GetJobs)

	router.GET("/api/jobs/:id", controllers.GetJobs)

	// admin := router.Group("/admin")
	// {
	// 	admin.GET("/api/job/list", controllers.GetJobsList)
	// }

	protected := router.Group("/")
	protected.Use(lib.AuthenticatedHandler)
	{
		protected.POST("/api/jobs/create", controllers.CreateJob)
		protected.POST("/api/jobs/update/:id", controllers.UpdateJob)
		protected.GET("/api/user/profile", controllers.GetProfile)
		protected.POST("/api/user/update", controllers.UpdateProfile)
		protected.POST("/api/user/change-password", controllers.ChangePassword)
		protected.POST("/api/proposals", controllers.CreateProposal)
		protected.PUT("/api/proposals/:id", controllers.UpdateProposal)
		protected.POST("/api/proposals/:id/withdraw", controllers.WithdrawProposal)
		protected.DELETE("/api/proposals/:id", controllers.DeleteProposal)
		protected.GET("/api/proposals/:id", controllers.GetProposal)
		protected.GET("/api/proposals/my", controllers.GetMyProposals)
	}

	log.Printf("Server starting on port %s", PORT)
	if err := router.Run("192.168.18.8:" + PORT); err != nil {
		log.Fatalf("failed to run server: %v", err)
	}

}
