package main

import (
	"fmt"
	"log"
	"tasksy/config"
	"tasksy/db"
	aws_services "tasksy/pkg"
	"tasksy/pkg/logger"
	"tasksy/pkg/worker"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"
)

func main() {
	cfg := config.LoadConfig()
	db, err := db.Connect(cfg.Database.URI)
	if err != nil {
		msg := fmt.Sprint("error while connecting to database %s", err.Error())
		logger.Log.Error(msg, zap.Error(err))
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

	log.Println(" [*] worker server started.")
	if err := srv.Run(mux); err != nil {
		msg := fmt.Sprintf("could not run server: %s", err.Error())
		logger.Log.Error(msg, zap.Error(err))
	}
}
