package main

import (
	"tasksy/config"
	"tasksy/db"
	"tasksy/internal/server"
	"tasksy/pkg/fcm"
	"tasksy/pkg/logger"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func main() {

	appConfig := config.LoadConfig()
	logger.InitLogger()
	redisClient := redis.NewClient(&redis.Options{Addr: appConfig.Redis.Addr})


	fcmClient, err := fcm.NewFCMClient()
	if err != nil {
		logger.Log.Error("failed to initialize FCM client", zap.Error(err))
		return
	}

	if err := db.ConnectDB(appConfig.Database.URI); err != nil {
		logger.Log.Error("failed to connect to database", zap.Error(err), zap.String("operation", "server-op"))
		return
	}

	if db.ApplyMigrations() != nil {
		logger.Log.Error("error while applying migrations", zap.Error(err), zap.String("db", "db-transaction"))
		return
	}

	func() {
		if err := db.DB.Raw("INSERT INTO system_settings (id, client_commission_percentage, freelancer_commission_percentage, application_fee_amount ) VALUES ('system_settings', 0, 0, 0) ON CONFLICT (id) DO NOTHING;").Error; err != nil {
			logger.Log.Error("error while initializing system settings", zap.Error(err), zap.String("operation", "server-op"))
			return
		}
		logger.Log.Info("system settings initialized")
	}()

	router := server.MakeRouter(db.DB, appConfig, fcmClient, redisClient)
	address := appConfig.Server.Addr

	logger.Log.Info("server starting", zap.String("address", address))
	defer logger.Sync()
	if err := router.Run(address); err != nil {
		logger.Log.Error("server stopped with error", zap.Error(err), zap.String("operation", "server-op"))
		return
	}
}
