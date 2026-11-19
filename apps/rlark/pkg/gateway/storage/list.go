package storage

import (
	"context"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// ObjectInfo holds information.
type ObjectInfo struct {
	Key          string    `json:"key"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified"`
	ETag         string    `json:"etag"`
	ContentType  string    `json:"content_type"`
}

// ListOptions holds options.
type ListOptions struct {
	Prefix    string
	MaxKeys   int
	Delimiter string
	Marker    string
}

// ListResult is an exported type.
type ListResult struct {
	Objects        []ObjectInfo `json:"objects"`
	CommonPrefixes []string     `json:"common_prefixes"`
	IsTruncated    bool         `json:"is_truncated"`
	NextMarker     string       `json:"next_marker"`
	MaxKeys        int          `json:"max_keys"`
}

// ListObjectsWithDelimiter lists the objectsWithDelimiter.
func (c *Client) ListObjectsWithDelimiter(options *ListOptions) (*ListResult, error) {
	ctx := context.Background()
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(c.getBucket()),
	}

	if options != nil {
		if options.Prefix != "" {
			input.Prefix = aws.String(options.Prefix)
		}
		if options.MaxKeys > 0 {
			input.MaxKeys = aws.Int32(int32(options.MaxKeys))
		}
		if options.Delimiter != "" {
			input.Delimiter = aws.String(options.Delimiter)
		}
		// Marker carries the opaque NextContinuationToken returned by a previous
		// page. It MUST be passed back via ContinuationToken (not StartAfter),
		// because the token is not an object key — using it as StartAfter would
		// produce incorrect or empty subsequent pages.
		if options.Marker != "" {
			input.ContinuationToken = aws.String(options.Marker)
		}
	}

	output, err := c.s3Client.ListObjectsV2(ctx, input)
	if err != nil {
		return nil, err
	}

	objects := make([]ObjectInfo, 0, len(output.Contents))
	for _, obj := range output.Contents {
		info := convertToObjectInfo(obj)
		// Filter out the directory placeholder object: when listing a prefix
		// such as "foo/" with delimiter "/", OSS/S3 may return a zero-byte
		// object whose key equals the prefix (the folder marker itself). It
		// represents the folder, not a file within it, so exclude it from the
		// file listing to avoid confusing the caller.
		if options != nil && options.Prefix != "" && info.Key == options.Prefix {
			continue
		}
		objects = append(objects, info)
	}

	var commonPrefixes []string
	for _, prefix := range output.CommonPrefixes {
		if prefix.Prefix != nil {
			commonPrefixes = append(commonPrefixes, *prefix.Prefix)
		}
	}

	var nextMarker string
	if aws.ToBool(output.IsTruncated) && output.NextContinuationToken != nil {
		nextMarker = *output.NextContinuationToken
	}

	return &ListResult{
		Objects:        objects,
		CommonPrefixes: commonPrefixes,
		IsTruncated:    aws.ToBool(output.IsTruncated),
		NextMarker:     nextMarker,
		MaxKeys:        options.MaxKeys,
	}, nil
}

// ListObjects lists the objects.
func (c *Client) ListObjects(options *ListOptions) ([]ObjectInfo, error) {
	result, err := c.ListObjectsWithDelimiter(options)
	if err != nil {
		return nil, err
	}
	return result.Objects, nil
}

// ListAllObjects lists the allObjects.
func (c *Client) ListAllObjects() ([]ObjectInfo, error) {
	return c.ListObjects(nil)
}

// ListObjectsByPrefix lists the objectsByPrefix.
func (c *Client) ListObjectsByPrefix(prefix string) ([]ObjectInfo, error) {
	options := &ListOptions{
		Prefix: prefix,
	}
	return c.ListObjects(options)
}

// IsObjectExist reports whether objectExist.
func (c *Client) IsObjectExist(objectKey string) (bool, error) {
	ctx := context.Background()
	_, err := c.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.getBucket()),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		// Check if it's a NotFound error
		if isNotFoundError(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Helper to check if error is a not-found type.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	// AWS SDK v2 typically returns *types.NotFound
	_, ok := err.(*types.NotFound)
	return ok
}

// GetObjectMeta returns the objectMeta.
func (c *Client) GetObjectMeta(objectKey string) (*ObjectInfo, error) {
	ctx := context.Background()
	output, err := c.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.getBucket()),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		return nil, err
	}

	var size int64
	if output.ContentLength != nil {
		size = *output.ContentLength
	}

	return &ObjectInfo{
		Key:          objectKey,
		Size:         size,
		LastModified: aws.ToTime(output.LastModified),
		ETag:         aws.ToString(output.ETag),
		ContentType:  aws.ToString(output.ContentType),
	}, nil
}

// To suppress unused import warning.
var _ = strconv.ParseInt
