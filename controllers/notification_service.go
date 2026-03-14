package controllers

import "tasksy/internal/modules/notifications"

var notificationService *notifications.Service

func SetNotificationService(service *notifications.Service) {
	notificationService = service
}
