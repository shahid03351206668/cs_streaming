package main

import (
	"fmt"
	"log"
	"tasksy/config"
	"tasksy/db"
	"tasksy/pkg/logger"
	"tasksy/server"

	"go.uber.org/zap"
)

func SetupConfig() {
}

func main() {
	appConfig := config.LoadConfig()
	logger.InitLogger()

	if err := db.ConnectDB(appConfig.Database.URI); err != nil {
		logger.Log.Error("failed to connect to database", zap.Error(err), zap.String("operation", "server-op"))
		return
	}

	fmt.Println(db.DB)
	router := server.MakeRouter(db.DB, appConfig)
	address := appConfig.Server.Addr

	log.Printf("Server starting on address %s", address)
	if err := router.Run(address); err != nil {
		logger.Log.Error("Error while starting server", zap.Error(err), zap.String("operation", "server-op"))
		return
	}

}
