package controllers

import "tasksy/internal/modules/user"

var userService *user.Service

func SetUserService(service *user.Service) {
	userService = service
}
