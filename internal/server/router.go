package server

import (
	"tasksy/config"
	"tasksy/controllers"
	"tasksy/internal/modules/chat"
	"tasksy/internal/modules/job"
	"tasksy/internal/modules/user"
	"tasksy/middleware"
	aws_services "tasksy/pkg"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func MakeRouter(db *gorm.DB, appConfig *config.Config) *gin.Engine {
	router := gin.Default()
	router.Static("/media", "./media")
	router.Use(middleware.LoggerMiddleware())

	s3Client := aws_services.NewS3Client(appConfig)

	corsConfig := cors.DefaultConfig()
	corsConfig.AllowAllOrigins = true
	corsConfig.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "Authorization"}
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
	router.Use(cors.New(corsConfig))

	router.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "pong"})
	})

	userService := user.NewService(db, appConfig, s3Client)
	userHandler := user.NewHandler(userService)
	jobPostService := job.NewService(db, s3Client)
	jobPostHandler := job.NewHandler(jobPostService)

	authRoutes := router.Group("/api/auth")
	{
		authRoutes.POST("/register", userHandler.RegisterUser)
		authRoutes.POST("/login", controllers.LoginController)
		authRoutes.POST("/verify-user", controllers.VerifyUser)
		authRoutes.POST("/refresh", controllers.RefreshTokenController)
		authRoutes.POST("/google-auth", controllers.GoogleSignInFirebaseController)
	}

	proposalRoutes := router.Group("/api/proposals")
	proposalRoutes.Use(middleware.AuthMiddleware())
	{
		proposalRoutes.POST("/:id/decision", controllers.ManageProposalDecision)
	}

	jobRoutes := router.Group("/api/jobs")

	jobRoutes.Use(middleware.AuthMiddleware())
	{
		jobRoutes.POST("/create", jobPostHandler.CreateJobPost)
		jobRoutes.GET("/my", controllers.GetMyJobs)
		jobRoutes.GET("/proposals/my", controllers.GetMyProposals)
		jobRoutes.POST("/send-proposal", controllers.CreateProposal)

		// Wildcard routes come LAST
		jobRoutes.GET("/:id/contract", controllers.GetContracts)
		jobRoutes.GET("/:id/proposal", controllers.GetJobProposals)
		// jobRoutes.POST("/create", controllers.CreateJobPost)
		jobRoutes.POST("/update/:id", controllers.UpdateJob)
	}

	publicRoutes := router.Group("/api/v1")
	{
		// /api/v1/user/:id/profile
		userGroup := publicRoutes.Group("/user/:id")
		{
			userGroup.GET("/profile", userHandler.GetUserProfile)
			userGroup.GET("/portfolio", userHandler.GetPortfolio)
			userGroup.GET("/certifications", userHandler.GetCertifications)

			protected := userGroup.Use(middleware.AuthMiddleware())
			{
				protected.PUT("portfolio", userHandler.UpdatePortfolio)
				protected.POST("/portfolio", userHandler.AddPortfolio)
				protected.DELETE("/portfolio", userHandler.DeletePortfolio)
			}
			// GetCertifications
		}

		publicRoutes.GET("/category/list", controllers.GetCategories)
		publicRoutes.GET("/category/:id", controllers.GetCategoryByID)
		publicRoutes.GET("/job/feed", jobPostHandler.JobFeedHandler)
		publicRoutes.GET("/job/:id", controllers.GetJobDetail)
		publicRoutes.PUT("/category/update/:id", controllers.UpdateCategory)
		publicRoutes.POST("/category/create", controllers.CreateCategory)
		publicRoutes.DELETE("/category/delete/:id", controllers.DeleteCategory)
	}

	protected := router.Group("/")
	protected.Use(middleware.AuthMiddleware())
	{
		protected.GET("/api/user/profile", controllers.GetProfile)
		protected.POST("/api/user/verify-credentials", controllers.VerifyUserCredential)
		protected.POST("/api/user/update", controllers.UpdateProfile)
		protected.POST("/api/user/change-password", controllers.ChangePassword)

		protected.GET("/api/proposals/get-contract", controllers.GetContracts)
		protected.POST("/api/proposals/create-contract", controllers.CreateContract)

		protected.GET("/api/proposals/:id", controllers.GetProposal)
		protected.POST("/api/proposals/:id/withdraw", controllers.WithdrawProposal)
		protected.PUT("/api/proposals/:id", controllers.UpdateProposal)
		protected.DELETE("/api/proposals/:id", controllers.DeleteProposal)

		protected.GET("/api/contracts/list", controllers.GetContracts)
		protected.POST("/api/contracts/:id/complete", controllers.CompleteContract)
	}

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
		chatProtected.GET("/:id/history", chatHandler.GetChatHistory)
		chatProtected.POST("/:id/message", chatHandler.SendMessage)
	}

	return router
}
