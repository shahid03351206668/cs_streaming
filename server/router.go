package server

import (
	"tasksy/config"
	"tasksy/controllers"
	"tasksy/internal/modules/chat"
	"tasksy/internal/modules/job"
	"tasksy/internal/modules/user"
	"tasksy/middleware"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func MakeRouter(db *gorm.DB, appConfig *config.Config) *gin.Engine {

	router := gin.Default()
	router.Use(middleware.LoggerMiddleware())

	corsConfig := cors.DefaultConfig()
	corsConfig.AllowAllOrigins = true
	corsConfig.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "Authorization"}
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
	router.Use(cors.New(corsConfig))

	router.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "pong"})
	})

	userService := user.NewService(db)
	userHandler := user.NewHandler(userService)
	jobPostService := job.NewService(db)
	jobPostHandler := job.NewHandler(jobPostService)

	// --- Auth Routes ---
	authRoutes := router.Group("/api/auth")
	{
		authRoutes.POST("/register", controllers.RegisterUser)
		authRoutes.POST("/login", controllers.LoginController)
		authRoutes.POST("/verify-user", controllers.VerifyUser)
		authRoutes.POST("/refresh", controllers.RefreshTokenController)
		authRoutes.POST("/google-auth", controllers.GoogleSignInFirebaseController)
	}

	// --- Proposal Routes ---
	proposalRoutes := router.Group("/api/proposals")
	proposalRoutes.Use(middleware.AuthMiddleware())
	{
		proposalRoutes.POST("/:id/decision", controllers.ManageProposalDecision)
	}

	// --- Job Routes ---
	jobRoutes := router.Group("/api/jobs")
	jobRoutes.Use(middleware.AuthMiddleware())
	{
		// ✅ FIXED: Static routes come FIRST
		jobRoutes.GET("/my", controllers.GetMyJobs)
		jobRoutes.GET("/proposals/my", controllers.GetMyProposals)

		// Wildcard routes come LAST
		jobRoutes.GET("/:id/contract", controllers.GetContracts)
		jobRoutes.GET("/:id/proposal", controllers.GetJobProposals)
		jobRoutes.POST("/create", controllers.CreateJob)
		jobRoutes.POST("/update/:id", controllers.UpdateJob)
	}

	// --- Public Routes ---
	publicRoutes := router.Group("/api/v1")
	{
		publicRoutes.GET("/user/:id/profile", userHandler.GetUserProfile)

		// ✅ FIXED: "create" and "list" moved ABOVE ":id"
		publicRoutes.POST("/category/create", controllers.CreateCategory)
		publicRoutes.GET("/category/list", controllers.GetCategories)
		publicRoutes.GET("/category/:id", controllers.GetCategoryByID)
		publicRoutes.DELETE("/category/delete/:id", controllers.DeleteCategory)
		publicRoutes.PUT("/category/update/:id", controllers.UpdateCategory)

		// ✅ FIXED: "feed" moved ABOVE ":id"
		publicRoutes.GET("/job/feed", jobPostHandler.JobFeedHandler)
		publicRoutes.GET("/job/:id", controllers.GetJobDetail)
	}

	// --- Protected Routes ---
	protected := router.Group("/")
	protected.Use(middleware.AuthMiddleware())
	{
		protected.GET("/api/user/profile", controllers.GetProfile)
		protected.POST("/api/user/verify-credentials", controllers.VerifyUserCredential)
		protected.POST("/api/user/update", controllers.UpdateProfile)
		protected.POST("/api/user/change-password", controllers.ChangePassword)

		// ✅ FIXED: "get-contract" moved ABOVE ":id"
		protected.GET("/api/proposals/get-contract", controllers.GetContracts)
		protected.POST("/api/proposals/create-contract", controllers.CreateContract)

		// Wildcards for proposals
		protected.GET("/api/proposals/:id", controllers.GetProposal)
		protected.POST("/api/proposals/:id/withdraw", controllers.WithdrawProposal)
		protected.PUT("/api/proposals/:id", controllers.UpdateProposal)
		protected.DELETE("/api/proposals/:id", controllers.DeleteProposal)

		// Contracts
		protected.GET("/api/contracts/list", controllers.GetContracts)
		protected.POST("/api/contracts/:id/complete", controllers.CompleteContract)
	}

	// --- Chat Routes ---
	chatRepo := chat.NewRepository(db)
	chatService := chat.NewService(chatRepo, appConfig)
	chatHandler := chat.NewHandler(chatService)

	chatGroup := router.Group("/chat")
	chatGroup.GET("/chat/ws", chatHandler.WSHandler) // Note: path is /chat/chat/ws here?

	chatProtected := chatGroup.Group("/")
	chatProtected.Use(middleware.AuthMiddleware())
	{
		chatProtected.GET("/inbox", chatHandler.GetInbox)
		chatProtected.POST("/init", chatHandler.InitiateChat)
		// Wildcards last
		chatProtected.GET("/:id/history", chatHandler.GetChatHistory)
		chatProtected.POST("/:id/message", chatHandler.SendMessage)
	}

	return router
}
