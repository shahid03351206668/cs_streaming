package server

// func NewRouter(db *gorm.DB) *gin.Engine {
//     r := gin.Default()

//     // 1. Initialize Modules
//     chatRepo := chat.NewRepository(db)
//     chatService := chat.NewService(chatRepo) // Service holds the active connections
//     chatHandler := chat.NewHandler(chatService)

//     // 2. Register Routes
//     api := r.Group("/api")
//     {
//         // Chat Routes
//         c := api.Group("/chat")
//         c.Use(middleware.AuthMiddleware()) 
//         {
//             c.POST("/init", chatHandler.InitiateChat)
//             c.GET("/inbox", chatHandler.GetInbox) // Add this to Handler interface
//             c.POST("/:id/message", chatHandler.SendMessage)
//         }
//     }

//     // WebSocket Route (No Auth Middleware here usually, handle via Query Token)
//     r.GET("/ws/chat", chatHandler.WSHandler)

//     return r
// }