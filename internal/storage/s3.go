package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AnkitSinha0/HashVault/internal/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Client wraps the AWS S3 SDK for presigned URL generation and object
// deletion. Points at MinIO in dev (S3Config.Endpoint set) and real AWS S3
// in prod (Endpoint empty — SDK uses the default AWS endpoint resolution).
type S3Client struct {
	client  *s3.Client
	presign *s3.PresignClient
	bucket  string
}

func NewS3Client(ctx context.Context, cfg *config.Config) (*S3Client, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.S3.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.S3.AccessKeyID, cfg.S3.SecretAccessKey, "",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("loading aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.S3.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.S3.Endpoint)
			// MinIO requires path-style addressing (bucket.endpoint won't resolve).
			o.UsePathStyle = true
		}
	})

	s3Client := &S3Client{
		client:  client,
		presign: s3.NewPresignClient(client),
		bucket:  cfg.S3.Bucket,
	}

	if err := s3Client.ensureBucket(ctx); err != nil {
		return nil, err
	}

	return s3Client, nil
}

// ensureBucket creates the bucket if it doesn't exist yet — convenient for
// local MinIO where nothing pre-provisions it. No-ops against real AWS S3
// once the bucket already exists (BucketAlreadyOwnedByYou is swallowed).
func (s *S3Client) ensureBucket(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.bucket)})
	if err == nil {
		return nil
	}

	_, err = s.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(s.bucket)})
	if err != nil {
		var alreadyOwned *types.BucketAlreadyOwnedByYou
		var alreadyExists *types.BucketAlreadyExists
		if errors.As(err, &alreadyOwned) || errors.As(err, &alreadyExists) {
			return nil
		}
		return fmt.Errorf("creating bucket %q: %w", s.bucket, err)
	}
	return nil
}

// PresignPutURL returns a URL the client can PUT the file bytes to directly.
func (s *S3Client) PresignPutURL(ctx context.Context, key string, expires time.Duration) (string, error) {
	req, err := s.presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expires))
	if err != nil {
		return "", fmt.Errorf("presigning put url: %w", err)
	}
	return req.URL, nil
}

// PresignGetURL returns a URL the client can GET the file bytes from directly.
func (s *S3Client) PresignGetURL(ctx context.Context, key string, expires time.Duration) (string, error) {
	req, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expires))
	if err != nil {
		return "", fmt.Errorf("presigning get url: %w", err)
	}
	return req.URL, nil
}

// DeleteObject removes the object from the bucket. Called when a
// StorageObject's ref_count reaches zero.
func (s *S3Client) DeleteObject(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("deleting s3 object: %w", err)
	}
	return nil
}

var _ ObjectStore = (*S3Client)(nil)
