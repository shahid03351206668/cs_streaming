package lib

// func UploadFile() {
// 	ext := filepath.Ext(file.Filename)
// 			baseFileName := strings.TrimSuffix(file.Filename, ext)
// 			baseFileName = filepath.Base(baseFileName)
// 			fileName := fmt.Sprintf("%s_%d%s", baseFileName, time.Now().UnixNano(), ext)
// 			filePath := filepath.Join(MEDIA_FILE_PATH, fileName)

// 			fmt.Println(filePath)

// 			if err := c.SaveUploadedFile(file, filePath); err != nil {
// 				tx.Rollback()
// 				c.JSON(http.StatusInternalServerError, gin.H{
// 					"message": "error",
// 					"error":   fmt.Sprintf("failed to upload file: %v", err),
// 				})
// 				return
// 			}

// 			jobMedia := models.JobMedia{
// 				JobID:     jobPost.ID,
// 				URL:       filePath,
// 				MediaType: getMediaType(file),
// 				FileName:  file.Filename,
// 				FileSize:  file.Size,
// 			}

// 			if err := tx.Create(&jobMedia).Error; err != nil {
// 				tx.Rollback()
// 				os.Remove(filePath)
// 				c.JSON(http.StatusInternalServerError, gin.H{
// 					"message": "error",
// 					"error":   "failed to save media record",
// 				})
// 				return
// 			}
// }
