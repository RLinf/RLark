package storage

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// IsBucketExist reports whether bucketExist.
func (c *Client) IsBucketExist() (bool, error) {
	ctx := context.Background()
	_, err := c.s3Client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(c.getBucket()),
	})
	if err != nil {
		// Check if bucket not found
		return false, err
	}
	return true, nil
}

// CreateBucket creates the bucket.
func (c *Client) CreateBucket() error {
	ctx := context.Background()
	_, err := c.s3Client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(c.getBucket()),
	})
	return err
}

// GetBucketInfo returns basic bucket info.
func (c *Client) GetBucketInfo() (string, error) {
	ctx := context.Background()
	output, err := c.s3Client.GetBucketLocation(ctx, &s3.GetBucketLocationInput{
		Bucket: aws.String(c.getBucket()),
	})
	if err != nil {
		return "", err
	}

	location := ""
	if output.LocationConstraint != "" {
		location = string(output.LocationConstraint)
	}
	return location, nil
}

// SetBucketACL sets the bucketACL.
func (c *Client) SetBucketACL(acl types.BucketCannedACL) error {
	ctx := context.Background()
	_, err := c.s3Client.PutBucketAcl(ctx, &s3.PutBucketAclInput{
		Bucket: aws.String(c.getBucket()),
		ACL:    acl,
	})
	return err
}

// GetBucketACL returns the bucketACL.
func (c *Client) GetBucketACL() (types.AccessControlPolicy, error) {
	ctx := context.Background()
	output, err := c.s3Client.GetBucketAcl(ctx, &s3.GetBucketAclInput{
		Bucket: aws.String(c.getBucket()),
	})
	if err != nil {
		return types.AccessControlPolicy{}, err
	}

	policy := types.AccessControlPolicy{}
	if output.Owner != nil {
		policy.Owner = output.Owner
	}
	policy.Grants = output.Grants
	return policy, nil
}

// SetBucketPublicRead sets the bucketPublicRead.
func (c *Client) SetBucketPublicRead() error {
	return c.SetBucketACL(types.BucketCannedACLPublicRead)
}

// SetBucketPrivate sets the bucketPrivate.
func (c *Client) SetBucketPrivate() error {
	return c.SetBucketACL(types.BucketCannedACLPrivate)
}
