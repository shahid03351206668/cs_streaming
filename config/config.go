package config

import (
	// "fmt"
	"log"
	"os"

	// "tasksy/models"

	// "github.com/golang/vscode-go/survey"
	"github.com/joho/godotenv"
)

type Config struct {
	Server         ServerConfig
	Database       DatabaseConfig
	AWS            AWSConfig
	Stripe         StripeConfig
	SystemSettings SystemSettings
}

type ServerConfig struct {
	Host string
	Port string
	Addr string
}

type DatabaseConfig struct {
	URI string
}

type AWSConfig struct {
	AccessKeyID     string
	SecretAccessKey string
	Region          string
	BucketName      string
	BucketURL       string
}

type StripeConfig struct {
	WebhookSecret string
	SecretKey     string
	APIKey        string
}

type SystemSettings struct {
	ClientCommissionPercentage     float64
	FreelancerCommissionPercentage float64
	ApplicationFeeAmount           int64
}

func LoadConfig() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("Info: No .env file found, relying on system environment variables")
	}

	host := getEnv("SERVER_HOST", "localhost")
	port := getEnv("SERVER_PORT", "8080")
	dbURI := getEnv("DB_URI", "")

	// fmt.Println("host, port")
	// fmt.Println(host, port)

	if dbURI == "" {
		log.Println("Warning: DB_URI is not set")
	}

	return &Config{
		// SystemSettings: SystemSettings{},
		Server: ServerConfig{
			Host: host,
			Port: port,
			Addr: host + ":" + port,
		},
		Database: DatabaseConfig{
			URI: dbURI,
		},
		Stripe: StripeConfig{
			WebhookSecret: getEnv("STRIPE_WEBHOOK_SIGNING_SECRET", ""),
			SecretKey:     getEnv("STRIPE_SECRET_KEY", ""),
			APIKey:        getEnv("STRIPE_API_KEY", ""),
		},
		AWS: AWSConfig{
			AccessKeyID:     getEnv("AWS_S3_USER_ACCESS_KEY_ID", ""),
			SecretAccessKey: getEnv("AWS_S3_USER_SECRET", ""),
			Region:          getEnv("AWS_S3_BUCKET_REGION", "eu-north-1"),
			BucketName:      getEnv("AWS_S3_BUCKET_NAME", ""),
			BucketURL:       getEnv("AWS_S3_BUCKET_URL", ""),
		},
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
