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
		log.Fatalf("Error while applying migrations: %v", err)
	}

	log.Println("Migrations applied successfully!")
}
