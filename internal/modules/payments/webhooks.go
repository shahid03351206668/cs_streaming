package payments

import (
	"tasksy/db"
	"tasksy/models"
)

var CACHED_SYSTEM_SETTINGS *models.SystemSettings

func InvalidateSettingsCache() {
	CACHED_SYSTEM_SETTINGS = nil
}

type PaymentHandler struct {
	service *PaymentService
}

func NewHandler(service *PaymentService) *PaymentHandler {
	return &PaymentHandler{service: service}
}

func GetSystemSettings() (*models.SystemSettings, error) {
	if CACHED_SYSTEM_SETTINGS == nil {
		var settings models.SystemSettings
		err := db.DB.First(&settings).Error
		if err == nil {
			CACHED_SYSTEM_SETTINGS = &settings
		}
		return &settings, err
	}
	return CACHED_SYSTEM_SETTINGS, nil
}
