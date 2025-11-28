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
	if err := DB.AutoMigrate(
		&models.User{},
		&models.Role{},
		&models.UserRoles{},
		&models.Category{},
		&models.JobPost{},
		&models.JobMedia{},
		&models.Proposal{},
		&models.ProposalAttachment{},
		&models.Contract{},
		&models.Payment{},
	); err != nil {
		log.Fatalf("Error while applying migrations: %v", err)
	}
}
