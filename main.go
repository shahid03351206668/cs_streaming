package main

import (
	"log"
	"net/http"
	"os"
	"tasksy/controllers"
	"tasksy/db"
	"tasksy/lib"
	"tasksy/middleware"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

type APIRoute struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

type Config struct {
	Host        string
	Port        string
	DatabaseURI string
}

func LoadConfig() Config {
	godotenv.Load()

	host := os.Getenv("SERVER_HOST")
	port := os.Getenv("SERVER_PORT")

	if host == "" {
		host = "localhost"
	}

	if port == "" {
		port = "8080"
	}

	dns := os.Getenv("DB_URI")

	return Config{
		Host:        host,
		Port:        port,
		DatabaseURI: dns,
	}
}
func SetupRouter() *gin.Engine {
	router := gin.Default()
	router.Use(cors.Default())
	router.Static("/media", "./media")
	return router
}

func MakeRoutes(router *gin.Engine) {
	authRoutes := router.Group("/api/auth")
	{
		authRoutes.GET("/list", controllers.GetUsers)
		authRoutes.POST("/register", controllers.RegisterUser)
		authRoutes.POST("/login", controllers.LoginController)
		authRoutes.POST("/verify-user", controllers.VerifyUser)
		authRoutes.POST("/refresh", controllers.RefreshTokenController)
		authRoutes.POST("/google-auth", controllers.GoogleSignInFirebaseController)
	}

	protectedRoutes := router.Group("/")
	protectedRoutes.Use(middleware.AuthMiddleware())
	{
		protectedRoutes.GET("/api/jobs/:id/contracts", controllers.GetContracts)
		protectedRoutes.POST("/api/jobs/create", controllers.CreateJob)
	}
}

func main() {
	config := LoadConfig()
	router := SetupRouter()

	if err := db.ConnectDB(config.DatabaseURI); err != nil {
		log.Fatalf("failed to connect to database: %v", err)
		return
	}

	if err := db.ApplyMigrations(); err != nil {
		log.Fatalf("failed to apply database migrations: %v", err)
		return
	}

	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusAccepted, gin.H{
			"message": "pong",
		})
	})

	MakeRoutes(router)

	router.GET("/api/routes", func(c *gin.Context) {
		ListRoutes(c, router)
	})

	router.GET("/api/user/list", controllers.GetUsers)
	router.GET("/api/category/list", controllers.GetCategories)
	router.POST("/api/category/create", controllers.CreateCategory)
	router.GET("/api/jobs/list", controllers.GetJobs)
	router.GET("/api/jobs/:id", controllers.GetJobDetail)

	protected := router.Group("/")
	protected.Use(lib.AuthenticatedHandler)

	{
		protected.GET("/api/user/profile", controllers.GetProfile)
		protected.GET("/api/proposals/:id", controllers.GetProposal)
		protected.GET("/api/jobs/my", controllers.GetMyJobs)
		protected.GET("/api/proposals/my", controllers.GetMyProposals)
		protected.GET("/api/jobs/:id/proposal", controllers.GetJobProposals)

		protected.POST("/api/user/verify-credentials", controllers.VerifyUserCredential)
		protected.POST("/api/jobs/update/:id", controllers.UpdateJob)
		protected.POST("/api/user/update", controllers.UpdateProfile)
		protected.POST("/api/user/change-password", controllers.ChangePassword)
		protected.POST("/api/proposals", controllers.CreateProposal)

		protected.POST("/api/proposals/:id/withdraw", controllers.WithdrawProposal)
		protected.PUT("/api/proposals/:id", controllers.UpdateProposal)
		protected.DELETE("/api/proposals/:id", controllers.DeleteProposal)

		protected.POST("/api/proposals/create-contract", controllers.CreateContract)
		protected.GET("/api/proposals/get-contract", controllers.GetContracts)
	}

	address := config.Host + ":" + config.Port
	log.Printf("Server starting on address %s", address)
	if err := router.Run(address); err != nil {
		log.Fatalf("failed to run server: %v", err)
	}

}

func ListRoutes(c *gin.Context, router *gin.Engine) {
	routes := make([]APIRoute, 0)
	for _, routeInfo := range router.Routes() {
		// Exclude the static media route and the route list endpoint itself for clarity, if desired
		// but generally, we include all public routes.
		if routeInfo.Path != "/media/*filepath" && routeInfo.Path != "/api/routes" {
			routes = append(routes, APIRoute{
				Method: routeInfo.Method,
				Path:   routeInfo.Path,
			})
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"total_routes": len(routes),
		"routes":       routes,
	})
}
