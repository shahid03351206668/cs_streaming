package utils

import (
	"fmt"
	"math/rand/v2"
	"mime/multipart"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

var letters = []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func GenerateRandomString(length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = letters[rand.Int64()%int64(len(letters))]
	}
	return string(b)
}

func FileType(file *multipart.FileHeader) string {
	ext := strings.ToLower(filepath.Ext(file.Filename))

	imageExts := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true}
	videoExts := map[string]bool{".mp4": true, ".mov": true, ".avi": true}
	docExts := map[string]bool{".pdf": true, ".doc": true, ".docx": true}

	switch {
	case imageExts[ext]:
		return "image"
	case videoExts[ext]:
		return "video"
	case docExts[ext]:
		return "document"
	default:
		return "other"
	}
}

func GetFileType(mimeType string) string {
	if strings.HasPrefix(mimeType, "image/") {
		return "image"
	}
	if strings.HasPrefix(mimeType, "video/") {
		return "video"
	}
	if strings.HasPrefix(mimeType, "audio/") {
		return "audio"
	}
	if strings.HasPrefix(mimeType, "application/pdf") {
		return "pdf"
	}
	return "file"
}
func GetMediaDuration(filePath string) int {
	cmd := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", filePath)
	output, err := cmd.Output()
	if err != nil {
		fmt.Println("Error getting duration:", err)
		return 0
	}

	durationStr := strings.TrimSpace(string(output))
	durationFloat, _ := strconv.ParseFloat(durationStr, 64)
	return int(durationFloat)
}
