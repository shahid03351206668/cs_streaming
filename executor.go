package executor

// import (
// 	"log"
// 	"os"

// 	"github.com/joho/godotenv"
// 	"gorm.io/driver/postgres"
// 	"gorm.io/gorm"
// )

// type FirebaseConfig struct {
// 	CredentialsFile string `mapstructure:"credentials_file"`
// }

// type RedisConfig struct {
// 	Addr string
// }

// type Config struct {
// 	Server         ServerConfig
// 	Database       DatabaseConfig
// 	Redis          RedisConfig
// 	AWS            AWSConfig
// 	Stripe         StripeConfig
// 	SystemSettings SystemSettings
// 	Firebase       FirebaseConfig
// }

// type ServerConfig struct {
// 	Host string
// 	Port string
// 	Addr string
// }

// type DatabaseConfig struct {
// 	URI string
// }

// type AWSConfig struct {
// 	AccessKeyID     string
// 	SecretAccessKey string
// 	Region          string
// 	BucketName      string
// 	BucketURL       string
// }

// type StripeConfig struct {
// 	WebhookSecret string
// 	SecretKey     string
// 	APIKey        string
// }

// type SystemSettings struct {
// 	ClientCommissionPercentage     float64
// 	FreelancerCommissionPercentage float64
// 	AppFeePercentage               float64
// 	ApplicationFeeAmount           int64
// 	ReferralDiscountPercentage     float64
// 	ReferralRewardAmount           int64
// }

// func LoadConfig() *Config {
// 	if err := godotenv.Load(); err != nil {
// 		log.Println("Info: No .env file found, relying on system environment variables")
// 	}

// 	host := getEnv("SERVER_HOST", "localhost")
// 	port := getEnv("SERVER_PORT", "5000")
// 	dbURI := getEnv("DB_URI", "")

// 	if dbURI == "" {
// 		log.Println("Warning: DB_URI is not set")
// 	}

// 	return &Config{
// 		Server: ServerConfig{
// 			Host: host,
// 			Port: port,
// 			Addr: host + ":" + port,
// 		},
// 		Database: DatabaseConfig{
// 			URI: dbURI,
// 		},
// 		Redis: RedisConfig{
// 			Addr: getEnv("REDIS_ADDR", "127.0.0.1:6379"),
// 		},
// 		Stripe: StripeConfig{
// 			WebhookSecret: getEnv("STRIPE_WEBHOOK_SIGNING_SECRET", ""),
// 			SecretKey:     getEnv("STRIPE_SECRET_KEY", ""),
// 			APIKey:        getEnv("STRIPE_API_KEY", ""),
// 		},
// 		AWS: AWSConfig{
// 			AccessKeyID:     getEnv("AWS_S3_USER_ACCESS_KEY_ID", ""),
// 			SecretAccessKey: getEnv("AWS_S3_USER_SECRET", ""),
// 			Region:          getEnv("AWS_S3_BUCKET_REGION", "eu-north-1"),
// 			BucketName:      getEnv("AWS_S3_BUCKET_NAME", ""),
// 			BucketURL:       getEnv("AWS_S3_BUCKET_URL", ""),
// 		},
// 		Firebase: FirebaseConfig{
// 			CredentialsFile: getEnv("FIREBASE_CONFIG", ""),
// 		},
// 	}
// }

// func getEnv(key, fallback string) string {
// 	if value, exists := os.LookupEnv(key); exists {
// 		return value
// 	}
// 	return fallback
// }

// func main() {

// 	config := LoadConfig()

// 	dbConfig := &gorm.Config{
// 		Logger:                                   logger.Default.LogMode(logger.Silent),
// 		DisableForeignKeyConstraintWhenMigrating: true,
// 	}

// 	DB, err = gorm.Open(postgres.Open(dsn), dbConfig)
// 	if err != nil {
// 		return err
// 	}
// // config.Database.URI
// 	// db :=
// }
