package main

import (
	"fmt"
	"log"
	"tasksy/config"
	"tasksy/db"
	"tasksy/internal/server"
	"tasksy/pkg/logger"

	"go.uber.org/zap"
)

func main() {
	appConfig := config.LoadConfig()
	logger.InitLogger()

	if err := db.ConnectDB(appConfig.Database.URI); err != nil {
		logger.Log.Error("failed to connect to database", zap.Error(err), zap.String("operation", "server-op"))
		return
	}

	err := db.ApplyMigrations()
	if err != nil {
		logger.Log.Error("error while applying migrations", zap.Error(err), zap.String("db", "db-transaction"))
		return
	}

	func() {
		if err := db.DB.Raw("INSERT INTO system_settings (id, client_commission_percentage, freelancer_commission_percentage, application_fee_amount ) VALUES ('system_settings', 0, 0, 0) ON CONFLICT (id) DO NOTHING;").Error; err != nil {
			logger.Log.Error("error while initializing system settings", zap.Error(err), zap.String("operation", "server-op"))
			return
		}
		fmt.Println("System Settings Initialized")
	}()

	router := server.MakeRouter(db.DB, appConfig)
	address := appConfig.Server.Addr

	log.Printf("Server starting on address %s", address)
	if err := router.Run(address); err != nil {
		logger.Log.Error("Error while starting server", zap.Error(err), zap.String("operation", "server-op"))
		return
	}
}
