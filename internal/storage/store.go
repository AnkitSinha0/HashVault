package storage

import (
	"context"
	"errors"
	"time"
)

// ErrObjectNotFound is returned by StatObject when the key doesn't exist.
var ErrObjectNotFound = errors.New("object not found")

// ObjectInfo is what the store itself recorded about an object. None of it
// comes from the client, which is the point: confirm checks these values,
// not the request body.
type ObjectInfo struct {
	Size int64
	// SHA256 is the base64 SHA-256 the store verified on upload. It is empty
	// when the client uploaded without the x-amz-checksum-sha256 header.
	SHA256 string
	ETag   string
}

// ObjectStore is the subset of S3 operations fileService depends on.
// *S3Client satisfies this; tests can substitute an in-memory fake instead
// of spinning up MinIO for every service-layer test.
type ObjectStore interface {
	// PresignPutURL signs a PUT for exactly size bytes. The caller must send
	// sha256B64 as the x-amz-checksum-sha256 header; the store rejects bytes
	// that don't hash to it.
	PresignPutURL(ctx context.Context, key string, size int64, sha256B64 string, expires time.Duration) (string, error)
	// PresignGetURL signs a GET that downloads as an attachment named filename.
	PresignGetURL(ctx context.Context, key, filename string, expires time.Duration) (string, error)
	StatObject(ctx context.Context, key string) (*ObjectInfo, error)
	// CopyObject copies src to dst only if src still has srcETag, so the
	// bytes that were verified are the bytes that get copied.
	CopyObject(ctx context.Context, src, dst, srcETag string) error
	DeleteObject(ctx context.Context, key string) error
}
