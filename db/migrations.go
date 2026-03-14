package db

import (
	"log"
	"tasksy/models"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func ConnectDB(dsn string) error {
	var err error

	dbConfig := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	}

	DB, err = gorm.Open(postgres.Open(dsn), dbConfig)
	if err != nil {
		return err
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	if err := sqlDB.Ping(); err != nil {
		return err
	}

	var version string
	if err := DB.Raw("SHOW server_version;").Scan(&version).Error; err == nil {
		log.Println("Database connected. Engine Version:", version)
	}

	return nil
}

func Connect(dsn string) (*gorm.DB, error) {
	var err error

	dbConfig := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	}

	DB, err = gorm.Open(postgres.Open(dsn), dbConfig)
	if err != nil {
		return nil, err
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	if err := sqlDB.Ping(); err != nil {
		return nil, err
	}

	var version string
	if err := DB.Raw("SHOW server_version;").Scan(&version).Error; err == nil {
		log.Println("Database connected. Engine Version:", version)
	}

	return DB, nil
}
func ApplyMigrations() error {
	modelsToMigrate := []interface{}{
		&models.SystemSettings{},
		&models.JobPostLocation{},
		&models.User{},
		&models.DeviceToken{},
		&models.Role{},
		&models.Permission{},
		&models.Category{},
		&models.JobPost{},
		&models.JobPostVideo{},
		&models.JobMedia{},
		&models.Proposal{},
		&models.ProposalAttachment{},
		&models.Review{},
		&models.Contract{},
		&models.PaymentTransaction{},
		&models.ChatConversation{},
		&models.ChatMessage{},
		&models.ChatAttachment{},
		&models.ChatParticipant{},
		&models.File{},
		&models.Portfolio{},
		&models.Certification{},
		&models.ReferralCode{},
		&models.ReferralUsage{},
		&models.NotificationPreference{},
		&models.NotificationPreferenceCategory{},
	}

	if err := DB.AutoMigrate(modelsToMigrate...); err != nil {
		return err
	}

	log.Println("Database migrations applied successfully")
	return nil
}
