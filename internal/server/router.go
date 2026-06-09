package server

import (
	"tasksy/config"
	"tasksy/controllers"
	"tasksy/internal/modules/chat"
	"tasksy/internal/modules/dispute"
	"tasksy/internal/modules/job"
	"tasksy/internal/modules/notifications"
	"tasksy/internal/modules/payments"
	paymentsv2 "tasksy/internal/modules/payments-v2"
	"tasksy/internal/modules/promotions"
	"tasksy/internal/modules/referrals"
	"tasksy/internal/modules/user"
	"tasksy/middleware"
	aws_services "tasksy/pkg"
	"tasksy/pkg/fcm"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

func MakeRouter(db *gorm.DB, appConfig *config.Config, fcmClient *fcm.FCMClient) *gin.Engine { //nolint:funlen
	router := gin.Default()
	router.Static("/media", "./media")

	s3Client := aws_services.NewS3Client(appConfig)
	notifService := notifications.NewService(db, fcmClient)
	notifHandler := notifications.NewHandler(notifService)
	controllers.SetNotificationService(notifService)

	redisOpt := asynq.RedisClientOpt{
		Addr: appConfig.Redis.Addr,
	}

	queueClient := asynq.NewClient(redisOpt)

	router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Length, Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	router.Use(middleware.LoggerMiddleware())
	testingRoutes := router.Group("/test")

	testingHandler := paymentsv2.NewHandler(appConfig, paymentsv2.NewService(db))
	testingRoutes.POST("webhooks/stripe/payment", testingHandler.HandleStripeWebhook)

	router.GET("/ping", func(c *gin.Context) {
		settings, _ := payments.GetSystemSettings()
		c.JSON(200, gin.H{"message": "pong", "settings": settings})
	})

	userService := user.NewService(db, appConfig, s3Client)
	userHandler := user.NewHandler(userService)
	jobPostService := job.NewService(db, s3Client, queueClient, notifService)
	jobPostHandler := job.NewHandler(jobPostService)

	ledgerService := payments.NewLedgerService(db)
	ledgerService.EnsureSystemAccounts()
	ledgerHandler := payments.NewLedgerHandler(ledgerService, db)

	payoutService := payments.NewPayoutService(db, &appConfig.Stripe, ledgerService)

	promotionService := promotions.NewService(db)
	promotionHandler := promotions.NewHandler(promotionService)

	disputeService := dispute.NewService(db, notifService, s3Client)
	disputeHandler := dispute.NewHandler(disputeService)
	// http://localhost:5679/api/v1/webhooks/stripe/payment

	paymentService := paymentsv2.NewService(db)
	paymentHandler := paymentsv2.NewHandler(appConfig, paymentService)

	router.POST("/api/v1/webhooks/stripe/payment", paymentHandler.HandleStripeWebhook)

	paymentRoutes := router.Group("/api/v1/payments")
	paymentRoutes.Use(middleware.AuthMiddleware())
	{
		paymentRoutes.POST("/add/user-account", paymentHandler.AddUserPaymentAccount)
		paymentRoutes.GET("/get/user-account", paymentHandler.GetUserAccount)
		paymentRoutes.POST("/payout/create", paymentHandler.HandleCreatePayout)
		paymentRoutes.GET("/wallet", paymentHandler.HandleUserWallet)
	}

	// Authenticated payment routes
	// paymentProtected := router.Group("/api/v1/payments")
	// paymentProtected.Use(middleware.AuthMiddleware())
	// {
	// 	// paymentProtected.GET("/transactions/my", paymentHandler.GetUserPaymentTransactions)
	// }

	// Wallet / withdrawal routes (authenticated)
	walletRoutes := router.Group("/api/v1/wallet")
	walletRoutes.Use(middleware.AuthMiddleware())
	{
		walletRoutes.GET("/balance", payoutService.GetWalletBalance)
		// walletRoutes.POST("/withdraw", payoutService.RequestPayout)
		// walletRoutes.GET("/withdrawals", payoutService.GetPayoutHistory)
		walletRoutes.GET("/bank-accounts", payoutService.ListBankAccounts)
		walletRoutes.POST("/bank-accounts", payoutService.AddBankAccount)
		walletRoutes.PUT("/bank-accounts/:id/default", payoutService.SetDefaultBankAccount)
		walletRoutes.DELETE("/bank-accounts/:id", payoutService.DeleteBankAccount)
	}

	referralHandler := referrals.NewHandler(referrals.NewService(db))

	// Referral routes (public)
	referralRoutes := router.Group("/api/v1/referrals")
	{
		referralRoutes.GET("/validate/:code", referralHandler.ValidateReferralCode)
	}

	// Referral routes (authenticated)
	referralProtected := router.Group("/api/v1/referrals")
	referralProtected.Use(middleware.AuthMiddleware())
	{
		referralProtected.POST("/codes", referralHandler.CreateReferralCode)
		referralProtected.GET("/codes/my", referralHandler.GetMyReferralCodes)
		referralProtected.GET("/codes/:id", referralHandler.GetReferralCodeByID)
		referralProtected.PUT("/codes/:id", referralHandler.UpdateReferralCode)
		referralProtected.DELETE("/codes/:id", referralHandler.DeleteReferralCode)
		referralProtected.GET("/my", referralHandler.GetMyReferrals)
		referralProtected.GET("/status", referralHandler.GetMyReferralStatus)
	}

	referralAdmin := router.Group("/api/v1/admin/referrals")
	referralAdmin.Use(middleware.AuthMiddleware())
	{
		referralAdmin.GET("/codes", referralHandler.GetAllReferralCodes)
		referralAdmin.GET("/usages", referralHandler.GetAllReferralUsages)
	}

	// Escrow routes (authenticated)
	// escrowRoutes := router.Group("/api/v1/escrow")
	// escrowRoutes.Use(middleware.AuthMiddleware())
	// {
	// 	escrowRoutes.POST("/contracts/:id/deposit", paymentHandler.InitiateEscrowDeposit)
	// 	escrowRoutes.GET("/contracts/:id/status", paymentHandler.GetEscrowStatus)
	// 	escrowRoutes.POST("/contracts/:id/refund", paymentHandler.RefundEscrow)
	// 	escrowRoutes.GET("/contracts/:id/summary", paymentHandler.GetPaymentSummaryWithPromotion)
	// }

	// Promotional offer routes (public list)
	router.GET("/api/v1/promotions", promotionHandler.ListActiveOffers)

	// Promotional offer routes (authenticated)
	promoProtected := router.Group("/api/v1/promotions")
	promoProtected.Use(middleware.AuthMiddleware())
	{
		promoProtected.GET("/my-eligibility", promotionHandler.CheckMyEligibility)
	}

	// Admin: users, jobs, settings, banks
	router.GET("/api/v1/admin/settings", controllers.GetSystemSettings)
	adminRoutes := router.Group("/api/v1/admin")
	// admin routes securuty middleware removed temporarly
	// adminRoutes.Use(middleware.AuthMiddleware())
	{
		adminRoutes.GET("/users", controllers.AdminUserListController)
		adminRoutes.GET("/users/:id", controllers.AdminGetUserController)
		adminRoutes.PUT("/users/:id", controllers.AdminUpdateUserController)
		adminRoutes.PUT("/users/:id/password", controllers.AdminChangeUserPasswordController)
		adminRoutes.GET("/users/:id/wallet", controllers.AdminUserWalletController)
		adminRoutes.GET("/jobs", controllers.AdminListJobsController)
		adminRoutes.GET("/jobs/:id", controllers.AdminGetJobDetailController)
		adminRoutes.PUT("/jobs/:id", controllers.AdminUpdateJobController)

		adminRoutes.GET("/payouts", payoutService.AdminListPayouts)
		// adminRoutes.GET("/payments/audit-logs", paymentHandler.GetPaymentAuditLogs)

		adminRoutes.GET("/ledger", ledgerHandler.GetAdminLedgerReport)
		adminRoutes.GET("/ledger/accounts/:id/balance", ledgerHandler.GetAccountBalanceHandler)

		adminRoutes.PUT("/settings", controllers.UpdateSystemSettings)
		adminRoutes.GET("/banks", controllers.AdminListBanks)
		adminRoutes.POST("/banks", controllers.AdminCreateBank)
		adminRoutes.PUT("/banks/:id", controllers.AdminUpdateBank)
		adminRoutes.DELETE("/banks/:id", controllers.AdminDeleteBank)
	}

	// Dispute routes (authenticated users)
	disputeRoutes := router.Group("/api/v1/disputes")
	disputeRoutes.Use(middleware.AuthMiddleware())
	{
		disputeRoutes.POST("", disputeHandler.CreateDispute)
		disputeRoutes.GET("", disputeHandler.ListMyDisputes)
		disputeRoutes.GET("/:id", disputeHandler.GetDispute)
	}

	// Dispute admin routes
	disputeAdmin := router.Group("/api/v1/admin/disputes")
	disputeAdmin.Use(middleware.AuthMiddleware())
	{
		disputeAdmin.GET("", disputeHandler.AdminListDisputes)
		disputeAdmin.GET("/:id", disputeHandler.AdminGetDispute)
		disputeAdmin.PUT("/:id/resolve", disputeHandler.AdminResolveDispute)
	}

	// Promotional offer admin routes
	promoAdmin := router.Group("/api/v1/admin/promotions")
	promoAdmin.Use(middleware.AuthMiddleware())
	{
		promoAdmin.POST("", promotionHandler.CreateOffer)
		promoAdmin.GET("", promotionHandler.ListOffers)
		promoAdmin.PUT("/:id", promotionHandler.UpdateOffer)
		promoAdmin.DELETE("/:id", promotionHandler.DeleteOffer)
	}

	authRoutes := router.Group("/api/auth")
	{
		// api/auth/reset-password
		authRoutes.POST("/reset-password", userHandler.ResetUserPassword)
		authRoutes.POST("/register", userHandler.RegisterUser)
		authRoutes.POST("/login", controllers.LoginController)
		authRoutes.POST("/user/auth-token", controllers.GetUserAuthToken)
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
		jobRoutes.DELETE("/:id", jobPostHandler.DeleteJobPost)
	}

	router.GET("/api/v1/get/system-settings", userHandler.GetSystemSettings)

	publicRoutes := router.Group("/api/v1")
	{
		userGroup := publicRoutes.Group("/user/:id")
		{
			userGroup.GET("/profile", userHandler.GetUserProfile)
			userGroup.GET("/portfolio", userHandler.GetPortfolio)
			userGroup.GET("/certifications", userHandler.GetCertifications)

			protected := userGroup.Use(middleware.AuthMiddleware())
			{
				protected.GET("/feed/preferences", userHandler.GetUserFeedPreferences)
				protected.POST("/feed/preferences", userHandler.UpdateUserFeedPreferences)
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
		publicRoutes.PUT("/category/reorder", controllers.ReorderCategories)
		publicRoutes.POST("/category/create", controllers.CreateCategory)
		publicRoutes.DELETE("/category/delete/:id", controllers.DeleteCategory)
		publicRoutes.GET("/banks", controllers.ListBanks)
	}

	protected := router.Group("/")
	protected.Use(middleware.AuthMiddleware())
	{
		protected.GET("/api/jobs/:id/payment-details", paymentHandler.GetJobPostPaymentDetails)
		protected.GET("/api/user/profile", controllers.GetProfile)
		protected.POST("/api/user/device-token", userHandler.SaveDeviceToken)
		protected.POST("/api/user/verify-credentials", controllers.VerifyUserCredential)
		protected.POST("/api/user/update", controllers.UpdateProfile)
		protected.POST("/api/user/change-password", controllers.ChangePassword)
		protected.GET("/api/v1/notifications/preferences", notifHandler.GetPreferences)
		protected.PUT("/api/v1/notifications/preferences", notifHandler.UpsertPreferences)

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
	chatService := chat.NewService(chatRepo, appConfig, notifService, db)
	chatHandler := chat.NewHandler(chatService)

	chatGroup := router.Group("/chat")
	chatGroup.GET("/chat/ws", chatHandler.WSHandler)
	chatProtected := chatGroup.Group("/")
	chatProtected.Use(middleware.AuthMiddleware())
	{
		chatProtected.GET("/inbox", chatHandler.GetInbox)
		chatProtected.POST("/init", chatHandler.InitiateChat)
		chatProtected.GET("/:id/history", chatHandler.GetChatHistory)
		chatProtected.GET("/:id/unread", chatHandler.GetUnreadMessages)
		chatProtected.POST("/:id/read", chatHandler.MarkConversationRead)
		chatProtected.POST("/:id/message", chatHandler.SendMessage)
	}

	return router
}
