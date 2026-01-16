package aws_services

import (
	// "container/ring"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"tasksy/config"
	"tasksy/pkg/logger"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"go.uber.org/zap"
)

const AWS_DEFAULT_REGION string = "eu-north-1"

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

func (c *S3Client) UploadFile(file io.Reader, filename, contentType, region, bucketName string) (string, string, error) {
	if c.client == nil {
		err := errors.New("S3 client is not initialized")
		logger.Log.Error("S3 client is not setup", zap.Error(err), zap.String("operation", "aws-s3-op"))
		return "", "", err
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
			return "", "", err
		}
	}

	folder := "files"
	keyString := fmt.Sprintf("%s/%d_%s", folder, time.Now().UnixNano(), filename)
	ObjectKey := aws.String(keyString)

	_, err := c.client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:      aws.String(targetBucket),
		Key:         ObjectKey,
		Body:        file,
		ContentType: aws.String(contentType),
		ACL:         types.ObjectCannedACLPublicRead, // <--- ADD THIS LINE
	})

	if err != nil {
		logger.Log.Error("Failed to upload to S3", zap.Error(err))
		return "", "", err
	}

	fileURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", targetBucket, targetRegion, keyString)
	return fileURL, keyString, nil
}

func (c *S3Client) UploadFileToBucket(file io.Reader, filename, contentType, bucketName string) (string, string, error) {
	region := c.appConfig.AWS.Region

	if region == "" {
		region = AWS_DEFAULT_REGION
	}

	fileName := strings.ReplaceAll(filename, " ", "_")
	key := fmt.Sprintf("files/%d_%s", time.Now().UnixNano(), fileName)

	_, err := c.client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:      aws.String(bucketName),
		Key:         aws.String(key),
		Body:        file,
		ContentType: aws.String(contentType),
		ACL:         types.ObjectCannedACLPublicRead,
	})
	if err != nil {
		logger.Log.Error("S3 Upload Failed", zap.Error(err))
		return "", "", err
	}
	fileURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", bucketName, region, key)
	return fileURL, key, nil
}

func (c *S3Client) DownloadFile(key, bucket, path string) error {
	if c.client == nil {
		err := errors.New("S3 client is not initialized")
		logger.Log.Error("S3 client is not setup", zap.Error(err), zap.String("operation", "aws-s3-op"))
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}

	outFile, err := os.Create(path)
	defer func() {
		outFile.Close()
	}()

	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}

	targetBucket := bucket
	if targetBucket == "" {
		targetBucket = c.appConfig.AWS.BucketName
	}

	output, err := c.client.GetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: aws.String(targetBucket),
		Key:    aws.String(key),
	})

	if err != nil {
		return fmt.Errorf("failed to download from s3: %w", err)
	}

	defer output.Body.Close()

	if _, err := io.Copy(outFile, output.Body); err != nil {
		return fmt.Errorf("failed to write file content: %w", err)
	}
	return nil
}
