package db

import (
	"log"
	"os"
	"tasksy/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

func ConnectDB() {
	dsn := os.Getenv("DB_URI")

	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})

	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
		return
	}

	log.Println("database connected")
}

func ApplyMigrations() {
	log.Println("Applying DB migrations...")

	if err := DB.AutoMigrate(&models.User{}); err != nil {
		log.Fatalf("Error while applying migration for User model: %v", err)
	}
	log.Println("User model migration applied successfully!")

	if err := DB.AutoMigrate(&models.Role{}); err != nil {
		log.Fatalf("Error while applying migration for Role model: %v", err)
	}
	log.Println("Role model migration applied successfully!")

	if err := DB.AutoMigrate(&models.UserRoles{}); err != nil {
		log.Fatalf("Error while applying migration for UserRoles model: %v", err)
	}
	log.Println("UserRoles model migration applied successfully!")

	log.Println("All migrations applied successfully!")
}
