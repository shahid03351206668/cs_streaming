package controllers

import (
	"net/http"
	"tasksy/db"
	"tasksy/models"

	"github.com/gin-gonic/gin"
)

func GetCategories(c *gin.Context) {
	type Response struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	
	var categories []Response

	err := db.DB.
		Model(&models.Category{}).
		Select("id", "name").
		Where("disable = ?", false).
		Scan(&categories).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error fetching categories",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    categories,
	})
}

func CreateCategory(c *gin.Context) {
	var DB = *db.DB

	var body struct {
		Name string `binding:"required" json:"name"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	result := DB.Create(&models.Category{
		Name: body.Name,
	})

	err := result.Error
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "success",
	})
}
