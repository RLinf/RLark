package storage

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
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

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		// 寻址方式与下发到数据面的 rclone 配置共用同一判定, 避免同一条存储记录
		// 在网关直连与挂载链路上使用不同的 URL 形式.
		o.UsePathStyle = EffectivePathStyle(config.Provider, config.UsePathStyle)
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
	})

	return &Client{
		config:        config,
		s3Client:      client,
		presignClient: s3.NewPresignClient(client),
	}, nil
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
