package services

import "errors"

var (
	ErrEmailTaken         = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInvalidToken       = errors.New("invalid or expired token")
	ErrStorageExceeded    = errors.New("storage quota exceeded")
	ErrInvalidChecksum    = errors.New("checksum must be 64 hex characters (SHA-256)")
	// ErrUploadNotVerified means confirm found no uploaded bytes, or bytes
	// whose size or SHA-256 doesn't match the request.
	ErrUploadNotVerified = errors.New("upload missing or does not match its checksum")
)
