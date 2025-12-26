package services

import (
	"context"
	"fmt"
	"io"
	"strings"
	"tasksy/config"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type AWSService struct {
	s3Client  *s3.Client
	appConfig *config.Config
}

func NewAWSService(appConfig *config.Config) *AWSService {
	creds := credentials.NewStaticCredentialsProvider(
		appConfig.AWS.AccessKeyID,
		appConfig.AWS.SecretAccessKey,
		"",
	)

	cfg, err := awsconfig.LoadDefaultConfig(context.TODO(),
		awsconfig.WithRegion(appConfig.AWS.Region),
		awsconfig.WithCredentialsProvider(creds),
	)

	var client *s3.Client
	if err == nil {
		client = s3.NewFromConfig(cfg)
	} else {
		fmt.Printf("AWS Config Error: %v\n", err)
	}
	return &AWSService{s3Client: client, appConfig: appConfig}
}

func (s *AWSService) UploadToS3(file io.Reader, filename, mimeType string) (string, error) {
	if s.s3Client == nil {
		return "", fmt.Errorf("S3 client not initialized")
	}

	region := s.appConfig.AWS.Region
	if region == "" {
		region = "eu-north-1"
	}

	key := fmt.Sprintf("chat/%d_%s", time.Now().UnixNano(), filename)
	bucketName := s.appConfig.AWS.BucketName

	_, err := s.s3Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:      aws.String(bucketName),
		Key:         aws.String(key),
		Body:        file,
		ContentType: aws.String(mimeType),
	})

	if err != nil {
		return "", err
	}

	if s.appConfig.AWS.BucketURL != "" {
		baseURL := strings.TrimRight(s.appConfig.AWS.BucketURL, "/")
		return fmt.Sprintf("%s/%s", baseURL, key), nil
	}

	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", bucketName, region, key), nil
}
