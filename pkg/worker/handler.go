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
	"tasksy/pkg/logger"
	"time"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type VideoProcessor struct {
	DB       *gorm.DB
	S3Client *aws_services.S3Client
}

func (processor *VideoProcessor) UploadHLSFolder(folderPath, s3FolderPrefix string) (string, error) {
	var masterURL string

	err := filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(folderPath, path)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)
		s3Key := fmt.Sprintf("%s/%s", s3FolderPrefix, relPath)

		contentType := "application/octet-stream"
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".m3u8":
			contentType = "application/x-mpegURL"
		case ".ts":
			contentType = "video/MP2T"
		}

		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open file %s: %w", path, err)
		}
		defer file.Close()

		url, err := processor.S3Client.UploadFileWithFixedKey(file, s3Key, contentType)
		if err != nil {
			return fmt.Errorf("failed to upload %s: %w", relPath, err)
		}

		if strings.HasSuffix(relPath, "master.m3u8") {
			masterURL = url
			logger.Log.Info("master playlist uploaded", zap.String("url", masterURL))
		}
		return nil
	})

	if err != nil {
		logger.Log.Error("HLS folder upload failed", zap.String("folder", folderPath), zap.Error(err))
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

	logger.Log.Info("processing video task", zap.String("job_id", p.JobID), zap.String("file", p.FileName))

	tempDir := filepath.Join(os.TempDir(), "worker_hls", p.JobID)
	os.MkdirAll(tempDir, 0755)
	defer os.RemoveAll(tempDir)

	tempFilePath := filepath.Join(tempDir, "downloaded.mp4")

	if err := processor.S3Client.DownloadFile(p.S3RawKey, "tasksy-raw-media", tempFilePath); err != nil {
		logger.Log.Error("failed to download raw video", zap.String("job_id", p.JobID), zap.String("key", p.S3RawKey), zap.Error(err))
		return fmt.Errorf("failed to download raw file: %v", err)
	}

	// Generate thumbnail
	thumbPath := filepath.Join(tempDir, "thumbnail.jpg")
	thumbnailURL := ""
	if err := processor.generateThumbnail(tempFilePath, thumbPath); err != nil {
		logger.Log.Warn("thumbnail generation skipped", zap.String("job_id", p.JobID), zap.Error(err))
	} else {
		thumbFile, err := os.Open(thumbPath)
		if err == nil {
			uniqueID := fmt.Sprintf("%d", time.Now().UnixNano())
			thumbKey := fmt.Sprintf("jobs/%s/thumbnails/%s.jpg", p.JobID, uniqueID)
			url, uploadErr := processor.S3Client.UploadFileWithFixedKey(thumbFile, thumbKey, "image/jpeg")
			thumbFile.Close()
			if uploadErr == nil {
				thumbnailURL = url
				logger.Log.Info("thumbnail uploaded", zap.String("job_id", p.JobID), zap.String("url", thumbnailURL))
			} else {
				logger.Log.Warn("thumbnail upload failed", zap.String("job_id", p.JobID), zap.Error(uploadErr))
			}
		}
	}

	hlsDir := filepath.Join(tempDir, "hls")
	if err := generateHLS(tempFilePath, hlsDir); err != nil {
		logger.Log.Error("HLS generation failed", zap.String("job_id", p.JobID), zap.Error(err))
		return err
	}

	uniqueID := fmt.Sprintf("%d", time.Now().UnixNano())
	s3Prefix := fmt.Sprintf("jobs/%s/video/%s", p.JobID, uniqueID)

	logger.Log.Info("uploading HLS folder", zap.String("job_id", p.JobID), zap.String("prefix", s3Prefix))
	masterURL, err := processor.UploadHLSFolder(hlsDir, s3Prefix)
	if err != nil {
		logger.Log.Error("HLS folder upload failed", zap.String("job_id", p.JobID), zap.Error(err))
		return err
	}
	logger.Log.Info("HLS folder uploaded", zap.String("job_id", p.JobID), zap.String("master_url", masterURL))

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
			logger.Log.Error("failed to update job status", zap.String("job_id", p.JobID), zap.Error(err))
			return err
		}
		return nil
	})

	if err != nil {
		logger.Log.Error("video task transaction failed", zap.String("job_id", p.JobID), zap.Error(err))
	} else {
		logger.Log.Info("video task completed", zap.String("job_id", p.JobID))
	}
	return err
}

func generateHLS(input string, output string) error {
	cmd := exec.Command("ffmpeg",
		"-y", "-i", input,
		"-threads", "0",
		"-filter_complex", "[0:v]split=2[v1][v2]; [v1]scale=w=1280:h=720:flags=lanczos[v1out]; [v2]scale=w=854:h=480:flags=lanczos[v2out]",
		"-map", "[v1out]", "-c:v:0", "libx264", "-preset", "fast", "-crf", "23",
		"-b:v:0", "2500k", "-maxrate:v:0", "2600k", "-bufsize:v:0", "5000k",
		"-profile:v:0", "high", "-level:v:0", "4.1",
		"-map", "[v2out]", "-c:v:1", "libx264", "-preset", "fast", "-crf", "24",
		"-b:v:1", "1000k", "-maxrate:v:1", "1200k", "-bufsize:v:1", "2000k",
		"-profile:v:1", "main", "-level:v:1", "3.1",
		"-map", "a:0", "-c:a:0", "aac", "-b:a:0", "128k", "-ac", "2",
		"-map", "a:0", "-c:a:1", "aac", "-b:a:1", "96k", "-ac", "2",
		"-f", "hls",
		"-hls_time", "6",
		"-hls_playlist_type", "vod",
		"-hls_flags", "independent_segments",
		"-hls_segment_type", "mpegts",
		"-master_pl_name", "master.m3u8",
		"-var_stream_map", "v:0,a:0 v:1,a:1",
		filepath.Join(output, "v%v", "stream.m3u8"),
	)

	logger.Log.Info("starting HLS generation", zap.String("input", input), zap.String("output", output))
	if out, err := cmd.CombinedOutput(); err != nil {
		logger.Log.Error("ffmpeg failed", zap.String("output", string(out)), zap.Error(err))
		return fmt.Errorf("ffmpeg output: %s, error: %v", string(out), err)
	}
	logger.Log.Info("HLS generation complete", zap.String("output", output))
	return nil
}
