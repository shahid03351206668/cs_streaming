package worker

import (
	"encoding/json"
	"github.com/hibiken/asynq"
)

const (
	TypeVideoTranscode = "video:transcode"
)

type VideoTranscodePayload struct {
	JobID       string
	S3RawKey    string
	FileName    string
	ContentType string
	FileSize    int64
}

func NewVideoTranscodeTask(jobID, s3Key, fileName, contentType string, size int64) (*asynq.Task, error) {
	payload, err := json.Marshal(VideoTranscodePayload{
		JobID:       jobID,
		S3RawKey:    s3Key,
		FileName:    fileName,
		ContentType: contentType,
		FileSize:    size,
	})

	if err != nil {
		return nil, err
	}

	return asynq.NewTask(TypeVideoTranscode, payload), nil
}
