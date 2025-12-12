package api

import (
	"net/http"
	"tasksy/db"
	"tasksy/models"

	"github.com/gin-gonic/gin"
)

func GetRole(c *gin.Context) {
	var roles []models.Role
	DB := db.DB
	results := DB.Find(&roles)

	if results.Error != nil {

		c.JSON(http.StatusInternalServerError, gin.H{"error": results.Error.Error(), "message": "error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": roles})
}

func CreateRole(c *gin.Context) {
	var body models.Role;
	
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "invalid input"})
		return
	}

	DB := db.DB

	err := DB.Create(&body).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": body})
}

func GetPermissions(c *gin.Context) {
	var permissions []models.Permission
	DB := db.DB

	results := DB.Find(&permissions)

	if results.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": results.Error.Error(), "message": "error"})
		return
	}
}
