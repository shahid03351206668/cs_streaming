package db

import (
	"fmt"
	"tasksy/models"
	applogger "tasksy/pkg/logger"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func ConnectDB(dsn string) error {
	var err error

	dbConfig := &gorm.Config{
		Logger:                                   logger.Default.LogMode(logger.Silent),
		DisableForeignKeyConstraintWhenMigrating: true,
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
		applogger.Log.Info("database connected", zap.String("engine_version", version))
	}

	return nil
}

func Connect(dsn string) (*gorm.DB, error) {
	var err error

	dbConfig := &gorm.Config{
		Logger:                                   logger.Default.LogMode(logger.Silent),
		DisableForeignKeyConstraintWhenMigrating: true,
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
		applogger.Log.Info("database connected", zap.String("engine_version", version))
	}

	return DB, nil
}
func ApplyMigrations() error {
	// Drop polymorphic FK constraints that conflict with the shared `files` table.
	// The files table is used by multiple entity types (portfolio media, dispute attachments, etc.)
	// so a per-entity foreign key referencing a single entity table would fail on rows
	// that belong to other entity types.
	DB.Exec(`ALTER TABLE files DROP CONSTRAINT IF EXISTS fk_disputes_attachments`)
	DB.Exec(`ALTER TABLE files DROP CONSTRAINT IF EXISTS fk_portfolios_media`)

	modelsToMigrate := []interface{}{
		&models.SystemSettings{},
		&models.User{},
		&models.Portfolio{},
		&models.Permission{},
		&models.JobPost{},
		&models.JobPostLocation{},
		&models.DeviceToken{},
		&models.Role{},
		&models.EmailAccount{},
		&models.EmailTemplate{},
		&models.Category{},
		&models.PaymentTransactionV3{},
		&models.EscrowTransactionV3{},
		&models.UserAddress{},
		&models.JobMedia{},
		&models.Proposal{},
		&models.ProposalAttachment{},
		&models.Review{},
		&models.Contract{},
		&models.ChatConversation{},
		&models.ChatMessage{},
		&models.ChatAttachment{},
		&models.ChatParticipant{},
		&models.File{},
		&models.Certification{},
		&models.ReferralCode{},
		&models.ReferralUsage{},
		&models.NotificationPreference{},
		&models.NotificationPreferenceCategory{},
		&models.PromotionalOffer{},
		&models.UserBankAccount{},
		&models.Bank{},
		&models.Dispute{},
		&models.UserFeedPreferences{},
		&models.JobReport{},
	}

	if err := DB.AutoMigrate(modelsToMigrate...); err != nil {
		fmt.Println(err.Error())
		return err
	}

	applogger.Log.Info("database migrations applied successfully")
	return nil
}
