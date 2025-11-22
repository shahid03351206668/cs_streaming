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

	var version string
	DB.Raw("SHOW server_version;").Scan(&version)
	log.Println("DB VERSION:", version)
	log.Println("database connected")
}

func ApplyMigrations() {
	log.Println("Applying DB migrations...")

	if err := DB.AutoMigrate(&models.User{}); err != nil {
		log.Fatalf("Error while applying migration for User model: %v", err)
	}
	if err := DB.AutoMigrate(&models.Role{}); err != nil {
		log.Fatalf("Error while applying migration for Role model: %v", err)
	}
	if err := DB.AutoMigrate(&models.UserRoles{}); err != nil {
		log.Fatalf("Error while applying migration for UserRoles model: %v", err)
	}
	if err := DB.AutoMigrate(&models.Category{}); err != nil {
		log.Fatalf("Error while applying migrations for Category model: %v", err)
	}
	if err := DB.AutoMigrate(&models.JobPost{}); err != nil {
		log.Fatalf("Error while applying migrations for JobPost model: %v", err)
	}
	if err := DB.AutoMigrate(&models.JobMedia{}); err != nil {
		log.Fatalf("Error while applying migrations for JobMedia model: %v", err)
	}
	if err := DB.AutoMigrate(&models.Proposal{}); err != nil {
		log.Fatalf("Error while applying migrations for Proposal model: %v", err)
	}
	if err := DB.AutoMigrate(&models.ProposalAttachment{}); err != nil {
		log.Fatalf("Error while applying migrations for ProposalAttachment model: %v", err)
	}

	log.Println("All migrations applied successfully!")
}
