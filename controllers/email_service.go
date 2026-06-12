package controllers

import "tasksy/internal/modules/email"

var emailService *email.Service

func SetEmailService(service *email.Service) {
	emailService = service
}
