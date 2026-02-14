package server

import (
	"tasksy/config"
	"tasksy/controllers"
	"tasksy/internal/modules/chat"
	"tasksy/internal/modules/job"
	"tasksy/internal/modules/payments"
	"tasksy/internal/modules/user"
	"tasksy/middleware"
	aws_services "tasksy/pkg"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

func MakeRouter(db *gorm.DB, appConfig *config.Config) *gin.Engine {
	router := gin.Default()
	router.Static("/media", "./media")
	router.Use(middleware.LoggerMiddleware())

	s3Client := aws_services.NewS3Client(appConfig)

	redisOpt := asynq.RedisClientOpt{
		Addr: "127.0.0.1:6379",
	}

	queueClient := asynq.NewClient(redisOpt)

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
	jobPostService := job.NewService(db, s3Client, queueClient)
	jobPostHandler := job.NewHandler(jobPostService)

	paymentService := payments.NewService(&appConfig.Stripe, db)
	paymentHandler := payments.NewHandler(paymentService)

	router.POST("/api/v1/webhooks/stripe/payment", paymentHandler.HandlePaymentIntents)

	paymentRoutes := router.Group("/api/v1/payments")
	{
		paymentRoutes.GET("/transactions", paymentHandler.GetPaymentTransactions)
		paymentRoutes.GET("/transactions/:id", paymentHandler.GetPaymentTransactionByID)
	}

	// Authenticated payment routes
	paymentProtected := router.Group("/api/v1/payments")
	paymentProtected.Use(middleware.AuthMiddleware())
	{
		paymentProtected.GET("/transactions/my", paymentHandler.GetUserPaymentTransactions)
	}

	// Referral routes (public)
	referralRoutes := router.Group("/api/v1/referrals")
	{
		referralRoutes.GET("/validate/:code", paymentHandler.ValidateReferralCode)
	}

	// Referral routes (authenticated)
	referralProtected := router.Group("/api/v1/referrals")
	referralProtected.Use(middleware.AuthMiddleware())
	{
		referralProtected.POST("/codes", paymentHandler.CreateReferralCode)
		referralProtected.GET("/codes/my", paymentHandler.GetMyReferralCodes)
		referralProtected.GET("/codes/:id", paymentHandler.GetReferralCodeByID)
		referralProtected.PUT("/codes/:id", paymentHandler.UpdateReferralCode)
		referralProtected.DELETE("/codes/:id", paymentHandler.DeleteReferralCode)
		referralProtected.GET("/my", paymentHandler.GetMyReferrals)
		referralProtected.GET("/status", paymentHandler.GetMyReferralStatus)

	}

	referralAdmin := router.Group("/api/v1/admin/referrals")
	referralAdmin.Use(middleware.AuthMiddleware())
	{
		referralAdmin.GET("/codes", paymentHandler.GetAllReferralCodes)
		referralAdmin.GET("/usages", paymentHandler.GetAllReferralUsages)
	}

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
		proposalRoutes.GET("/:id/payment-summary", paymentHandler.GetProposalPaymentDetails)
	}

	jobRoutes := router.Group("/api/job")
	jobRoutes.Use(middleware.AuthMiddleware())
	{
		jobRoutes.POST("/create", jobPostHandler.CreateJobPost)
		jobRoutes.GET("/my", controllers.GetMyJobs)
		jobRoutes.GET("/proposals/my", controllers.GetMyProposals)
		jobRoutes.POST("/send-proposal", controllers.CreateProposal)

		jobRoutes.GET("/:id/contract", controllers.GetContracts)
		jobRoutes.GET("/:id/proposal", controllers.GetJobProposals)
		jobRoutes.POST("/update/:id", controllers.UpdateJob)
	}

	publicRoutes := router.Group("/api/v1")
	{
		userGroup := publicRoutes.Group("/user/:id")
		{
			userGroup.GET("/profile", userHandler.GetUserProfile)
			userGroup.GET("/portfolio", userHandler.GetPortfolio)
			userGroup.GET("/certifications", userHandler.GetCertifications)

			protected := userGroup.Use(middleware.AuthMiddleware())
			{
				protected.POST("/certifications", userHandler.AddCertification)
				protected.PUT("/certifications", userHandler.UpdateCertification)
				protected.DELETE("/certifications", userHandler.DeleteCertification)
				protected.PUT("portfolio", userHandler.UpdatePortfolio)
				protected.POST("/portfolio", userHandler.AddPortfolio)
				protected.DELETE("/portfolio", userHandler.DeletePortfolio)
			}
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
	chatGroup.GET("/chat/ws", chatHandler.WSHandler)
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
