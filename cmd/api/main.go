package api

import (
	"tasksy/config"
	"tasksy/middleware"
	"tasksy/pkg/logger"
	"tasksy/services"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func SetupConfig() {
}

func main() {
	logger.InitLogger()
	router := gin.Default()
	appConfig := config.LoadConfig()
	router.Use(middleware.LoggerMiddleware())

	// awsService :=
	services.NewAWSService(appConfig)

	corsConfig := cors.DefaultConfig()
	corsConfig.AllowAllOrigins = true
	corsConfig.AllowHeaders = []string{
		"Origin",
		"Content-Length",
		"Content-Type",
		"Authorization",
	}

	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}

	router.Use(cors.New(corsConfig))
}
