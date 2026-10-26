package lib

// func UploadFile() {
// 	ext := filepath.Ext(file.Filename)
// 			baseFileName := strings.TrimSuffix(file.Filename, ext)
// 			baseFileName = filepath.Base(baseFileName)
// 			fileName := fmt.Sprintf("%s_%d%s", baseFileName, time.Now().UnixNano(), ext)
// 			filePath := filepath.Join(MEDIA_FILE_PATH, fileName)

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
