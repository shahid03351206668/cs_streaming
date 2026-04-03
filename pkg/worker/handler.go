package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"tasksy/models"
	aws_services "tasksy/pkg"
	"time"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

type VideoProcessor struct {
	DB       *gorm.DB
	S3Client *aws_services.S3Client
}

func (processor *VideoProcessor) UploadHLSFolder(folderPath, s3FolderPrefix string) (string, error) {
	var masterURL string

	// 1. Walk through the directory recursively
	err := filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
		fmt.Println(path)

		if err != nil {
			return err
		}
		// Skip directories, we only upload files
		if info.IsDir() {
			return nil
		}

		// 2. Calculate S3 Key (Relative Path)
		// This strips the local temp dir prefix.
		// Example: /tmp/hls/123/v0/segment.ts -> v0/segment.ts
		relPath, err := filepath.Rel(folderPath, path)
		if err != nil {
			return err
		}

		// CRITICAL: Normalize path separators.
		// On Windows, relPath might be "v0\segment.ts", but S3 requires "v0/segment.ts".
		relPath = filepath.ToSlash(relPath)

		// Combine with the prefix (e.g., "jobs/job_123/video/456/v0/segment.ts")
		s3Key := fmt.Sprintf("%s/%s", s3FolderPrefix, relPath)

		// 3. Determine Content-Type
		// Browsers strictly require these MIME types for HLS playback.
		contentType := "application/octet-stream"
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".m3u8":
			contentType = "application/x-mpegURL"
		case ".ts":
			contentType = "video/MP2T"
		}

		// 4. Open File
		fmt.Println("opening file ", path)
		file, err := os.Open(path)

		if err != nil {
			return fmt.Errorf("failed to open file %s: %w", path, err)
		}
		defer file.Close()

		// 5. Upload to S3

		fmt.Println("Uploading file ", file.Name(), s3Key, contentType)
		fmt.Println("Uploading file ", file.Name(), s3Key, contentType)

		// FIX: Use the new function that doesn't add timestamps
		url, err := processor.S3Client.UploadFileWithFixedKey(file, s3Key, contentType)
		fmt.Println(url)
		if err != nil {
			return fmt.Errorf("failed to upload %s: %w", relPath, err)
		}
		fmt.Println("file uploaded")

		// 6. Capture Master Playlist URL
		// We need to return this specific URL so it can be saved in the database.
		// Note: Ensure your FFmpeg command names the main file "master.m3u8"

		if strings.HasSuffix(relPath, "master.m3u8") {
			masterURL = url
		}
		fmt.Println(masterURL)
		return nil
	})

	if err != nil {
		fmt.Println(err.Error())
		return "", err
	}

	if masterURL == "" {
		return "", fmt.Errorf("master.m3u8 was not found in the generated folder")
	}

	return masterURL, nil
}

func (processor *VideoProcessor) generateThumbnail(inputPath, outputPath string) error {
	cmd := exec.Command("ffmpeg",
		"-y", "-i", inputPath,
		"-ss", "00:00:01",
		"-vframes", "1",
		"-vf", "scale=1280:-2",
		"-q:v", "2",
		outputPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("thumbnail generation failed: %s, error: %v", string(out), err)
	}
	return nil
}

func (processor *VideoProcessor) HandleVideoTask(ctx context.Context, t *asynq.Task) error {
	var p VideoTranscodePayload

	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("json unmarshal failed: %v", err)
	}

	fmt.Printf(" [x] Processing Video for Job: %s\n", p.JobID)
	tempDir := filepath.Join(os.TempDir(), "worker_hls", p.JobID)
	os.MkdirAll(tempDir, 0755)
	defer os.RemoveAll(tempDir)

	tempFilePath := filepath.Join(tempDir, "downloaded.mp4")

	err := processor.S3Client.DownloadFile(p.S3RawKey, "tasksy-raw-media", tempFilePath)
	if err != nil {
		return fmt.Errorf("failed to download raw file: %v", err)
	}

	// Generate thumbnail
	thumbPath := filepath.Join(tempDir, "thumbnail.jpg")
	thumbnailURL := ""
	if err := processor.generateThumbnail(tempFilePath, thumbPath); err != nil {
		fmt.Printf("thumbnail generation warning: %v\n", err)
	} else {
		thumbFile, err := os.Open(thumbPath)
		if err == nil {
			uniqueID := fmt.Sprintf("%d", time.Now().UnixNano())
			thumbKey := fmt.Sprintf("jobs/%s/thumbnails/%s.jpg", p.JobID, uniqueID)
			url, uploadErr := processor.S3Client.UploadFileWithFixedKey(thumbFile, thumbKey, "image/jpeg")
			thumbFile.Close()
			if uploadErr == nil {
				thumbnailURL = url
				fmt.Printf("thumbnail uploaded: %s\n", thumbnailURL)
			}
		}
	}

	hlsDir := filepath.Join(tempDir, "hls")

	if err := generateHLS(tempFilePath, hlsDir); err != nil {
		return err
	}

	uniqueID := fmt.Sprintf("%d", time.Now().UnixNano())
	s3Prefix := fmt.Sprintf("jobs/%s/video/%s", p.JobID, uniqueID)

	fmt.Println("uploading hls folder")
	masterURL, err := processor.UploadHLSFolder(hlsDir, s3Prefix)
	fmt.Println("hls folder uploaded")

	if err != nil {
		fmt.Println(err.Error())
		return err
	}

	err = processor.DB.Transaction(func(tx *gorm.DB) error {
		media := models.JobMedia{
			JobID:     p.JobID,
			URL:       masterURL,
			MediaType: "application/x-mpegURL",
			FileName:  p.FileName,
			FileSize:  p.FileSize,
			Thumbnail: thumbnailURL,
		}

		if err := tx.Create(&media).Error; err != nil {
			return err
		}

		if err := tx.Model(models.JobPost{}).Where("id = ?", p.JobID).Update("status", models.JobStatusOpen).Error; err != nil {
			fmt.Println(err.Error())
			return err
		}
		return nil
	})

	return err
}

func generateHLS(input string, output string) error {
	cmd := exec.Command("ffmpeg",
		"-y", "-i", input,
		"-threads", "0",
		"-filter_complex", "[0:v]split=2[v1][v2]; [v1]scale=w=1280:h=720:flags=lanczos[v1out]; [v2]scale=w=854:h=480:flags=lanczos[v2out]",
		// 720p stream
		"-map", "[v1out]", "-c:v:0", "libx264", "-preset", "fast", "-crf", "23",
		"-b:v:0", "2500k", "-maxrate:v:0", "2600k", "-bufsize:v:0", "5000k",
		"-profile:v:0", "high", "-level:v:0", "4.1",
		// 480p stream
		"-map", "[v2out]", "-c:v:1", "libx264", "-preset", "fast", "-crf", "24",
		"-b:v:1", "1000k", "-maxrate:v:1", "1200k", "-bufsize:v:1", "2000k",
		"-profile:v:1", "main", "-level:v:1", "3.1",
		// Audio (both streams)
		"-map", "a:0", "-c:a:0", "aac", "-b:a:0", "128k", "-ac", "2",
		"-map", "a:0", "-c:a:1", "aac", "-b:a:1", "96k", "-ac", "2",
		// HLS settings
		"-f", "hls",
		"-hls_time", "6",
		"-hls_playlist_type", "vod",
		"-hls_flags", "independent_segments",
		"-hls_segment_type", "mpegts",
		"-master_pl_name", "master.m3u8",
		"-var_stream_map", "v:0,a:0 v:1,a:1",
		filepath.Join(output, "v%v", "stream.m3u8"),
	)

	fmt.Println("generating HLS streams")

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg output: %s, error: %v", string(out), err)
	}
	fmt.Println("HLS generation complete")
	return nil
}
