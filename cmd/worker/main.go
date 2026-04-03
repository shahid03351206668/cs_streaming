package main

import (
	"tasksy/config"
	"tasksy/db"
	aws_services "tasksy/pkg"
	"tasksy/pkg/logger"
	"tasksy/pkg/worker"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"
)

func main() {
	logger.InitLogger()

	cfg := config.LoadConfig()

	db, err := db.Connect(cfg.Database.URI)
	if err != nil {
		logger.Log.Error("failed to connect to database", zap.Error(err))
	}
	s3 := aws_services.NewS3Client(cfg)

	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: "localhost:6379"},
		asynq.Config{
			Concurrency: 50,
		},
	)
	processor := &worker.VideoProcessor{DB: db, S3Client: s3}
	mux := asynq.NewServeMux()
	mux.HandleFunc(worker.TypeVideoTranscode, processor.HandleVideoTask)

	logger.Log.Info("worker server started", zap.String("queue", worker.TypeVideoTranscode), zap.Int("concurrency", 50))
	defer logger.Sync()
	if err := srv.Run(mux); err != nil {
		logger.Log.Error("worker server stopped with error", zap.Error(err))
	}
}
