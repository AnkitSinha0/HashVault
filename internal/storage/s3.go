package storage

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/AnkitSinha0/HashVault/internal/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// StagingPrefix holds uploads that haven't been confirmed yet. Objects under
// it expire after stagingExpiryDays, so an upload that is never confirmed
// doesn't occupy storage forever.
const (
	StagingPrefix     = "staging/"
	stagingExpiryDays = 1
)

// S3Client wraps the AWS S3 SDK for presigned URL generation and object
// management. Points at MinIO in dev (S3Config.Endpoint set) and real AWS S3
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
	if err := s3Client.expireStaging(ctx); err != nil {
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

// expireStaging installs a lifecycle rule that deletes unconfirmed uploads.
// Idempotent: the same rule is written on every boot.
func (s *S3Client) expireStaging(ctx context.Context) error {
	_, err := s.client.PutBucketLifecycleConfiguration(ctx, &s3.PutBucketLifecycleConfigurationInput{
		Bucket: aws.String(s.bucket),
		LifecycleConfiguration: &types.BucketLifecycleConfiguration{
			Rules: []types.LifecycleRule{{
				ID:         aws.String("expire-unconfirmed-uploads"),
				Status:     types.ExpirationStatusEnabled,
				Filter:     &types.LifecycleRuleFilter{Prefix: aws.String(StagingPrefix)},
				Expiration: &types.LifecycleExpiration{Days: aws.Int32(stagingExpiryDays)},
			}},
		},
	})
	if err != nil {
		return fmt.Errorf("setting staging lifecycle rule: %w", err)
	}
	return nil
}

// PresignPutURL returns a URL the client can PUT exactly size bytes to.
// Content-Length is part of the signature, so a different size is refused.
// The checksum header is not signed (the SDK leaves it out), so the store
// only verifies it when the client sends it; StatObject is what confirm
// trusts.
func (s *S3Client) PresignPutURL(ctx context.Context, key string, size int64, sha256B64 string, expires time.Duration) (string, error) {
	req, err := s.presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:            aws.String(s.bucket),
		Key:               aws.String(key),
		ContentLength:     aws.Int64(size),
		ChecksumSHA256:    aws.String(sha256B64),
		ChecksumAlgorithm: types.ChecksumAlgorithmSha256,
	}, s3.WithPresignExpires(expires))
	if err != nil {
		return "", fmt.Errorf("presigning put url: %w", err)
	}
	return req.URL, nil
}

// PresignGetURL returns a URL the client can GET the file bytes from directly.
// The response is forced to an opaque attachment, so a stored file can never
// render as a page on the storage origin, whatever bytes it holds.
func (s *S3Client) PresignGetURL(ctx context.Context, key, filename string, expires time.Duration) (string, error) {
	req, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket:                     aws.String(s.bucket),
		Key:                        aws.String(key),
		ResponseContentType:        aws.String("application/octet-stream"),
		ResponseContentDisposition: aws.String("attachment; filename*=UTF-8''" + url.PathEscape(filename)),
	}, s3.WithPresignExpires(expires))
	if err != nil {
		return "", fmt.Errorf("presigning get url: %w", err)
	}
	return req.URL, nil
}

// StatObject reports the size and verified SHA-256 the store holds for key.
func (s *S3Client) StatObject(ctx context.Context, key string) (*ObjectInfo, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket:       aws.String(s.bucket),
		Key:          aws.String(key),
		ChecksumMode: types.ChecksumModeEnabled,
	})
	if err != nil {
		var notFound *types.NotFound
		var noSuchKey *types.NoSuchKey
		if errors.As(err, &notFound) || errors.As(err, &noSuchKey) {
			return nil, ErrObjectNotFound
		}
		return nil, fmt.Errorf("stat s3 object: %w", err)
	}
	return &ObjectInfo{
		Size:   aws.ToInt64(out.ContentLength),
		SHA256: aws.ToString(out.ChecksumSHA256),
		ETag:   aws.ToString(out.ETag),
	}, nil
}

// CopyObject copies src to dst inside the bucket, failing if src no longer
// has srcETag (someone re-uploaded it after it was verified). The copy's
// metadata is replaced, so whatever Content-Type the uploader sent is dropped.
func (s *S3Client) CopyObject(ctx context.Context, src, dst, srcETag string) error {
	_, err := s.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:            aws.String(s.bucket),
		Key:               aws.String(dst),
		CopySource:        aws.String(s.bucket + "/" + src),
		CopySourceIfMatch: aws.String(srcETag),
		MetadataDirective: types.MetadataDirectiveReplace,
		ContentType:       aws.String("application/octet-stream"),
		ChecksumAlgorithm: types.ChecksumAlgorithmSha256,
	})
	if err != nil {
		return fmt.Errorf("copying s3 object: %w", err)
	}
	return nil
}

// DeleteObject removes the object from the bucket. Deleting a key that
// doesn't exist is not an error in S3.
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
