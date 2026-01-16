package utils

import (
	"fmt"
	"math/rand/v2"
	"mime/multipart"
	"os"
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

func GenerateHLS(inputPath string, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}

	// FFmpeg Command for Multi-Bitrate HLS
	// This creates two streams:
	// 1. 720p (High)
	// 2. 480p (Low)
	// And maps them to a single master.m3u8 file

	cmd := exec.Command("ffmpeg",
		"-y", "-i", inputPath,

		// --- Video Filter Complex: Split video into 2 versions ---
		"-filter_complex", "[0:v]split=2[v1][v2]; [v1]scale=w=1280:h=720[v1out]; [v2]scale=w=854:h=480[v2out]",

		// --- Stream 1: 720p ---
		"-map", "[v1out]", "-c:v:0", "libx264", "-b:v:0", "2500k", "-maxrate:v:0", "2600k", "-bufsize:v:0", "5000k",

		// --- Stream 2: 480p ---
		"-map", "[v2out]", "-c:v:1", "libx264", "-b:v:1", "1000k", "-maxrate:v:1", "1200k", "-bufsize:v:1", "2000k",

		// --- Audio: Map same audio to both streams ---
		"-map", "a:0", "-c:a", "aac", "-b:a", "128k", "-ac", "2",
		"-map", "a:0",

		// --- HLS Settings ---
		"-f", "hls",
		"-hls_time", "6", // 6 second chunks
		"-hls_playlist_type", "vod", // Video on Demand
		"-hls_flags", "independent_segments",
		"-master_pl_name", "master.m3u8", // The main file your API will return

		// --- Stream Mapping ---
		"-var_stream_map", "v:0,a:0 v:1,a:1",

		// --- Output Filename Format ---
		// Creates folders v0/ and v1/ for segments
		filepath.Join(outputDir, "v%v", "stream.m3u8"),
	)

	// Capture output for debugging if it fails
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg HLS generation failed: %s, output: %s", err, string(out))
	}

	return nil
}
