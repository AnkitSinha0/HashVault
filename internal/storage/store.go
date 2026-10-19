package storage

import (
	"context"
	"time"
)

// ObjectStore is the subset of S3 operations fileService depends on.
// *S3Client satisfies this; tests can substitute an in-memory fake instead
// of spinning up MinIO for every service-layer test.
type ObjectStore interface {
	PresignPutURL(ctx context.Context, key string, expires time.Duration) (string, error)
	PresignGetURL(ctx context.Context, key string, expires time.Duration) (string, error)
	DeleteObject(ctx context.Context, key string) error
}
