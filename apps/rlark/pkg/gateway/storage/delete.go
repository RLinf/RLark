package storage

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// DeleteObject deletes the object.
func (c *Client) DeleteObject(objectKey string) error {
	ctx := context.Background()
	_, err := c.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.getBucket()),
		Key:    aws.String(objectKey),
	})
	return err
}

// DeleteMultipleObjects deletes the multipleObjects.
func (c *Client) DeleteMultipleObjects(objectKeys []string) ([]string, error) {
	if len(objectKeys) == 0 {
		return nil, nil
	}

	ctx := context.Background()
	objects := make([]types.ObjectIdentifier, len(objectKeys))
	for i, key := range objectKeys {
		objects[i] = types.ObjectIdentifier{
			Key: aws.String(key),
		}
	}

	input := &s3.DeleteObjectsInput{
		Bucket: aws.String(c.getBucket()),
		Delete: &types.Delete{
			Objects: objects,
			Quiet:   aws.Bool(false),
		},
	}

	output, err := c.s3Client.DeleteObjects(ctx, input)
	if err != nil {
		return nil, err
	}

	return extractDeletedObjects(output), nil
}

// DeleteObjectsByPrefix deletes the objectsByPrefix.
func (c *Client) DeleteObjectsByPrefix(prefix string) ([]string, error) {
	objects, err := c.ListObjectsByPrefix(prefix)
	if err != nil {
		return nil, err
	}

	if len(objects) == 0 {
		return nil, nil
	}

	objectKeys := make([]string, len(objects))
	for i, obj := range objects {
		objectKeys[i] = obj.Key
	}

	return c.DeleteMultipleObjects(objectKeys)
}

// DeleteObjectIfExists deletes the objectIfExists.
func (c *Client) DeleteObjectIfExists(objectKey string) (bool, error) {
	exists, err := c.IsObjectExist(objectKey)
	if err != nil {
		return false, err
	}

	if !exists {
		return false, nil
	}

	err = c.DeleteObject(objectKey)
	if err != nil {
		return false, err
	}

	return true, nil
}
