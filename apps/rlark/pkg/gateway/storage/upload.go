package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

// UploadOptions holds options.
type UploadOptions struct {
	ObjectKey   string
	ContentType string
}

// UploadFile uploads the file.
func (c *Client) UploadFile(objectKey string, reader io.Reader) error {
	ctx := context.Background()
	input := &s3.PutObjectInput{
		Bucket: aws.String(c.getBucket()),
		Key:    aws.String(objectKey),
		Body:   reader,
	}

	setContentLength(input, reader)

	_, err := c.s3Client.PutObject(ctx, input)
	return err
}

// UploadFileFromBytes uploads the fileFromBytes.
func (c *Client) UploadFileFromBytes(objectKey string, data []byte) error {
	return c.UploadFile(objectKey, bytes.NewReader(data))
}

// 从 Content-Disposition 头中解析完整的文件名（包含路径）.
func parseFullFilename(header *multipart.FileHeader) string {
	contentDisposition := header.Header.Get("Content-Disposition")
	if contentDisposition == "" {
		return header.Filename
	}

	// 使用正则表达式匹配 filename="..."
	re := regexp.MustCompile(`filename="([^"]*)"`)
	matches := re.FindStringSubmatch(contentDisposition)
	if len(matches) > 1 {
		return matches[1]
	}

	// 如果正则匹配失败，尝试简单的字符串分割
	if strings.Contains(contentDisposition, "filename=") {
		parts := strings.Split(contentDisposition, "filename=")
		if len(parts) > 1 {
			filename := strings.Trim(parts[1], `"`)
			return filename
		}
	}

	// 兜底返回原始文件名
	return header.Filename
}

// UploadFileFromMultipart uploads the fileFromMultipart.
func (c *Client) UploadFileFromMultipart(fileHeader *multipart.FileHeader) (string, error) {
	return c.UploadFileFromMultipartWithOptions(fileHeader)
}

// UploadFileFromMultipartWithOptions uploads a file from a multipart form header, preserving the original filename.
func (c *Client) UploadFileFromMultipartWithOptions(fileHeader *multipart.FileHeader) (string, error) {
	file, err := fileHeader.Open()
	if err != nil {
		return "", err
	}
	defer utils.CloseIO(file)

	objectKey := parseFullFilename(fileHeader)

	ctx := context.Background()
	input := &s3.PutObjectInput{
		Bucket: aws.String(c.getBucket()),
		Key:    aws.String(objectKey),
		Body:   file,
	}

	setContentLength(input, file)

	_, err = c.s3Client.PutObject(ctx, input)
	if err != nil {
		return "", err
	}

	fmt.Printf("DEBUG: Upload completed successfully, returned key: %s\n", objectKey)
	return objectKey, nil
}

// UploadFileWithOptions uploads the fileWithOptions.
func (c *Client) UploadFileWithOptions(reader io.Reader, options UploadOptions) error {
	objectKey := options.ObjectKey
	if objectKey == "" {
		return ErrInvalidObjectKey
	}

	ctx := context.Background()
	input := &s3.PutObjectInput{
		Bucket: aws.String(c.getBucket()),
		Key:    aws.String(objectKey),
		Body:   reader,
	}

	if options.ContentType != "" {
		input.ContentType = aws.String(options.ContentType)
	}

	setContentLength(input, reader)

	_, err := c.s3Client.PutObject(ctx, input)
	return err
}

// setContentLength 当 reader 实现了 io.ReadSeeker 时，设置 ContentLength
// 这可以防止 AWS SDK v2 对 S3 兼容服务使用 STREAMING-UNSIGNED-PAYLOAD-TRAILER
// 阿里云 OSS 等 S3 兼容服务不支持此特性.
func setContentLength(input *s3.PutObjectInput, reader io.Reader) {
	if rs, ok := reader.(io.ReadSeeker); ok {
		if pos, err := rs.Seek(0, io.SeekEnd); err == nil {
			input.ContentLength = aws.Int64(pos)
			_, _ = rs.Seek(0, io.SeekStart)
		}
	}
}
