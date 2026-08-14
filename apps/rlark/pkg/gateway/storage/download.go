package storage

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

// Ensure v4 is used.
var _ = v4.PresignedHTTPRequest{}

// GetObject returns the object.
func (c *Client) GetObject(objectKey string) (io.ReadCloser, error) {
	ctx := context.Background()
	output, err := c.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.getBucket()),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		return nil, err
	}
	return output.Body, nil
}

// DownloadFile downloads the file.
func (c *Client) DownloadFile(objectKey, localFilePath string) error {
	body, err := c.GetObject(objectKey)
	if err != nil {
		return err
	}
	defer utils.CloseIO(body)

	f, err := os.Create(localFilePath)
	if err != nil {
		return err
	}
	defer utils.CloseIO(f)

	_, err = io.Copy(f, body)
	return err
}

// GetObjectBytes returns the objectBytes.
func (c *Client) GetObjectBytes(objectKey string) ([]byte, error) {
	body, err := c.GetObject(objectKey)
	if err != nil {
		return nil, err
	}
	defer utils.CloseIO(body)

	return io.ReadAll(body)
}

// GetObjectURL returns the objectURL.
func (c *Client) GetObjectURL(objectKey string, expireSeconds int64) (string, error) {
	ctx := context.Background()
	input := &s3.GetObjectInput{
		Bucket: aws.String(c.getBucket()),
		Key:    aws.String(objectKey),
	}

	presignedReq, err := c.presignClient.PresignGetObject(ctx, input, func(o *s3.PresignOptions) {
		o.Expires = time.Duration(expireSeconds) * time.Second
	})
	if err != nil {
		return "", err
	}

	return presignedReq.URL, nil
}

// GetTempObjectURL returns the tempObjectURL.
func (c *Client) GetTempObjectURL(objectKey string) (string, error) {
	return c.GetObjectURL(objectKey, 3600)
}

// SaveObjectToLocal saves the objectToLocal.
func (c *Client) SaveObjectToLocal(objectKey, localDir string) (string, error) {
	localFilePath := localDir + "/" + objectKey

	err := os.MkdirAll(localDir, 0755)
	if err != nil {
		return "", err
	}

	err = c.DownloadFile(objectKey, localFilePath)
	if err != nil {
		return "", err
	}

	return localFilePath, nil
}
