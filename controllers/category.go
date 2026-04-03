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

	var results []Response

	err := db.DB.
		Model(&models.Category{}).
		Select("id", "name").
		Where("disable = ?", false).
		Scan(&results).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error fetching categories",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    results,
	})
}

func CreateCategory(c *gin.Context) {
	DB := db.DB

	var body struct {
		Name string `binding:"required" json:"name"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error fucked",
			"error":   err.Error(),
		})
		return
	}

	exists := DB.Where("name = ?", body.Name).First(&models.Category{})
	if exists.RowsAffected > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "category with this name already exists",
		})
		return
	}

	var category = models.Category{Name: body.Name}
	results := DB.Create(&category)

	if results.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   results.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "success",
		"data":    category,
	})
}

func GetCategoryByID(c *gin.Context) {
	id := c.Param("id")

	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "id is required",
		})
		return
	}

	DB := db.DB

	var category models.Category
	err := DB.Where("id = ? ", id).First(&category).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error fetching category",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":    category,
		"message": "success",
	})

}

func UpdateCategory(c *gin.Context) {
	id := c.Param("id")


	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "id is required",
		})
		return
	}

	var body struct {
		Name    string `binding:"required" json:"name"`
		Disable bool   `json:"disable"`
		ID      string `json:"id"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	DB := db.DB

	var category models.Category
	err := DB.Model(&category).Where("id = ?", id).Updates(&body).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "error",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    category,
	})

}
func DeleteCategory(c *gin.Context) {
	id := c.Param("id")

	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "error",
			"error":   "id is required",
		})
		return
	}

	DB := db.DB

	DB.Where("id = ?", id).Delete(&models.Category{})

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
	})
}
