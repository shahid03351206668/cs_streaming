package aws_services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"tasksy/config"
	"tasksy/pkg/logger"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/zap"
)

type S3Client struct {
	client    *s3.Client
	appConfig *config.Config
}

func NewS3Client(appConfig *config.Config) *S3Client {
	var client *s3.Client

	creds := credentials.NewStaticCredentialsProvider(
		appConfig.AWS.AccessKeyID,
		appConfig.AWS.SecretAccessKey,
		"",
	)
	cfg, err := awsconfig.LoadDefaultConfig(context.TODO(),
		awsconfig.WithRegion(appConfig.AWS.Region),
		awsconfig.WithCredentialsProvider(creds),
	)

	if err == nil {
		client = s3.NewFromConfig(cfg)
	} else {
		fmt.Printf("AWS Config Error: %v\n", err)
	}
	return &S3Client{client: client, appConfig: appConfig}
}

func (c *S3Client) UploadFile(file io.Reader, filename, mimeType, region, bucketName string) (string, error) {
	if c.client == nil {
		err := errors.New("S3 client is not initialized")
		logger.Log.Error("S3 client is not setup", zap.Error(err), zap.String("operation", "aws-s3-op"))
		return "", err
	}

	targetRegion := region
	if targetRegion == "" {
		targetRegion = c.appConfig.AWS.Region
		if targetRegion == "" {
			targetRegion = "eu-north-1"
		}
	}

	targetBucket := bucketName
	if targetBucket == "" {
		targetBucket = c.appConfig.AWS.BucketName
		if targetBucket == "" {
			err := errors.New("bucket name is missing")
			logger.Log.Error("The bucket name has not been provided or configured.", zap.Error(err), zap.String("operation", "aws-s3-op"))
			return "", err
		}
	}

	folder := "files"
	key := fmt.Sprintf("%s/%d_%s", folder, time.Now().UnixNano(), filename)
	_, err := c.client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:      aws.String(targetBucket),
		Key:         aws.String(key),
		Body:        file,
		ContentType: aws.String(mimeType),
	})

	if err != nil {
		logger.Log.Error("Failed to upload to S3", zap.Error(err))
		return "", err
	}

	// S3 Bucket URL Format: https://BUCKET.s3.REGION.amazonaws.com/KEY
	fileURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", targetBucket, targetRegion, key)
	return fileURL, nil
}
