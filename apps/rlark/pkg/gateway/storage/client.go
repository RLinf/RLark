package storage

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/sirupsen/logrus"
)

// Client is a client.
type Client struct {
	config        *Config
	s3Client      *s3.Client
	presignClient *s3.PresignClient
}

// NewClient creates a new Client.
func NewClient(config *Config) (*Client, error) {
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(config.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			config.AccessKeyId,
			config.SecretAccessKey,
			"",
		)),
	)
	if err != nil {
		return nil, err
	}

	cfg.BaseEndpoint = aws.String(config.Endpoint)
	// 	cfg.BaseEndpoint = aws.String(fmt.Sprintf("https://%s.oss-%s.aliyuncs.com", config.Bucket, config.Region))
	// } else if config.Endpoint != "" {
	// 	cfg.BaseEndpoint = aws.String(config.Endpoint)
	// }

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = config.UsePathStyle
		if strings.EqualFold(config.Provider, "Alibaba") {
			o.UsePathStyle = false
		}
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
	})

	ossClient := &Client{
		config:        config,
		s3Client:      client,
		presignClient: s3.NewPresignClient(client),
	}

	go func() {
		logrus.Info("S3 client warming up...")
		_, err := ossClient.IsBucketExist()
		if err != nil {
			logrus.Warnf("S3 connection warmup failed: %v", err)
		} else {
			logrus.Info("S3 connection warmed up successfully")
		}
	}()

	return ossClient, nil
}

// NewClientWithDefault creates a new ClientWithDefault.
func NewClientWithDefault() (*Client, error) {
	return NewClient(DefaultConfig())
}

// getBucket returns the bucket name from config.
func (c *Client) getBucket() string {
	return c.config.Bucket
}

// Helper to convert types.Object to our ObjectInfo.
func convertToObjectInfo(obj types.Object) ObjectInfo {
	info := ObjectInfo{}
	if obj.Key != nil {
		info.Key = *obj.Key
	}
	if obj.Size != nil {
		info.Size = *obj.Size
	}
	if obj.LastModified != nil {
		info.LastModified = *obj.LastModified
	}
	if obj.ETag != nil {
		info.ETag = *obj.ETag
	}
	return info
}

// Helper function to extract deletion result.
func extractDeletedObjects(output *s3.DeleteObjectsOutput) []string {
	var deleted []string
	if output.Deleted != nil {
		for _, d := range output.Deleted {
			if d.Key != nil {
				deleted = append(deleted, *d.Key)
			}
		}
	}
	return deleted
}
