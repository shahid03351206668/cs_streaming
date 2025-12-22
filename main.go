package main

import (
	"fmt"
	"log"
	"net/http"
	"tasksy/api"
	"tasksy/config"
	"tasksy/controllers"
	"tasksy/db"
	"tasksy/internal/modules/chat"
	"tasksy/middleware"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

type APIRoute struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

func SetupRouter() *gin.Engine {
	router := gin.Default()
	config := cors.DefaultConfig()

	config.AllowAllOrigins = true
	config.AllowHeaders = []string{
		"Origin",
		"Content-Length",
		"Content-Type",
		"Authorization",
	}

	config.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
	router.Use(cors.New(config))
	router.Static("/media", "./media")
	return router
}

func MakeRoutes(router *gin.Engine) {
	adminRoutesV1 := router.Group("/api/v1/admin")
	{
		adminRoutesV1.GET("/roles", api.GetRole)
		adminRoutesV1.POST("/roles", api.CreateRole)
		adminRoutesV1.POST("/users", api.CreateUser)
		adminRoutesV1.GET("/users/:id", api.GetUser)
		// api/v1/admin/users
		// CreateRole
		// adminRoutesV1.POST("/roles", controllers.CreateRole)
		// adminRoutesV1.GET("/permissions", controllers.GetPermissions)
	}

	authRoutesV1 := router.Group("/api/v1/auth")
	{
		authRoutesV1.POST("/login", controllers.LoginControllerV1)
	}

	authRoutes := router.Group("/api/auth")
	{
		authRoutes.GET("/list", controllers.GetUsers)
		authRoutes.POST("/register", controllers.RegisterUser)
		authRoutes.POST("/login", controllers.LoginController)
		authRoutes.POST("/verify-user", controllers.VerifyUser)
		authRoutes.POST("/refresh", controllers.RefreshTokenController)
		authRoutes.POST("/google-auth", controllers.GoogleSignInFirebaseController)
	}

	jobRoutes := router.Group("/api/jobs")
	jobRoutes.Use(middleware.AuthMiddleware())

	{
		jobRoutes.GET("/my", controllers.GetMyJobs)
		jobRoutes.GET("/proposals/my", controllers.GetMyProposals)
		jobRoutes.GET("/:id/contract", controllers.GetContracts)
		jobRoutes.GET("/:id/proposal", controllers.GetJobProposals)
		jobRoutes.POST("/create", controllers.CreateJob)
		jobRoutes.POST("/update/:id", controllers.UpdateJob)

	}
	proposalRoutes := router.Group("/api/proposals")
	proposalRoutes.Use(middleware.AuthMiddleware())
	{
		proposalRoutes.POST("/:id/decision", controllers.ManageProposalDecision)
	}
}

func main() {
	appConfig := config.LoadConfig()
	router := SetupRouter()

	if err := db.ConnectDB(appConfig.Database.URI); err != nil {
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
	router.GET("/api/v1/category/:id", controllers.GetCategoryByID)
	router.POST("/api/v1/category/create", controllers.CreateCategory)
	router.GET("/api/v1/category/list", controllers.GetCategories)
	router.DELETE("/api/v1/category/delete/:id", controllers.DeleteCategory)
	router.PUT("/api/v1/category/update/:id", controllers.UpdateCategory)

	router.GET("/api/jobs/list", controllers.GetJobs)
	router.GET("/api/jobs/:id", controllers.GetJobDetail)

	adminRoutes := router.Group("/api/v1/")
	{
		adminRoutes.POST("resource/list/:model", api.GetResourceList)
		adminRoutes.GET("/users/list", controllers.AdminUserListController)
		adminRoutes.GET("/users/:id", controllers.AdminGetUserController)
	}

	protected := router.Group("/")
	protected.Use(middleware.AuthMiddleware())

	{
		protected.POST("/api/contracts/:id/review", controllers.AddReview)
		protected.GET("/api/user/profile", controllers.GetProfile)
		protected.GET("/api/proposals/:id", controllers.GetProposal)
		protected.POST("/api/user/verify-credentials", controllers.VerifyUserCredential)

		protected.POST("/api/user/update", controllers.UpdateProfile)
		protected.POST("/api/user/change-password", controllers.ChangePassword)
		protected.POST("/api/job/send-proposal", controllers.CreateProposal)

		protected.POST("/api/proposals/:id/withdraw", controllers.WithdrawProposal)
		protected.PUT("/api/proposals/:id", controllers.UpdateProposal)
		protected.DELETE("/api/proposals/:id", controllers.DeleteProposal)

		protected.POST("/api/proposals/create-contract", controllers.CreateContract)
		protected.POST("/api/contracts/:id/complete", controllers.CompleteContract)
		protected.GET("/api/contracts/list", controllers.GetContracts)
		protected.GET("/api/proposals/get-contract", controllers.GetContracts)
	}

	fmt.Println("registering chat routes")
	chatRepo := chat.NewRepository(db.DB)
	chatService := chat.NewService(chatRepo, appConfig)
	chatHandler := chat.NewHandler(chatService)
	chatRoutes := router.Group("/chat")

	router.GET("/chat/ws", chatHandler.WSHandler)

	chatRoutes.Use(middleware.AuthMiddleware())
	{
		chatRoutes.POST("/init", chatHandler.InitiateChat)
		chatRoutes.POST("/:id/message", chatHandler.SendMessage)
		chatRoutes.GET("/inbox", chatHandler.GetInbox)
		chatRoutes.GET("/:id/history", chatHandler.GetChatHistory)

	}

	address := appConfig.Server.Addr
	log.Printf("Server starting on address %s", address)
	if err := router.Run(address); err != nil {
		log.Fatalf("failed to run server: %v", err)
	}

}

func ListRoutes(c *gin.Context, router *gin.Engine) {
	routes := make([]APIRoute, 0)
	for _, routeInfo := range router.Routes() {
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
