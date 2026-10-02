package dto

import (
	"time"

	"github.com/google/uuid"
)

// InitUploadRequest is sent before any bytes are uploaded. The client hashes
// the file locally (SHA-256) so the server can check for a dedup hit before
// asking the client to upload anything.
type InitUploadRequest struct {
	FileName string     `json:"file_name" binding:"required,min=1,max=255"`
	MimeType string     `json:"mime_type" binding:"required"`
	Size     int64      `json:"size"      binding:"required,gt=0"`
	Checksum string     `json:"checksum"  binding:"required,len=64,hexadecimal"`
	FolderID *uuid.UUID `json:"folder_id"`
}

// InitUploadResponse tells the client whether it needs to upload bytes at
// all. Deduplicated=true means the content already exists in S3 — the
// client should call /files/confirm immediately without uploading.
type InitUploadResponse struct {
	UploadURL    string `json:"upload_url,omitempty"`
	Deduplicated bool   `json:"deduplicated"`
}

// ConfirmUploadRequest carries the same identifying fields as init so the
// service can create (or dedup-link) the File + StorageObject rows without
// trusting anything from the completed S3 PUT.
type ConfirmUploadRequest struct {
	FileName string     `json:"file_name" binding:"required,min=1,max=255"`
	MimeType string     `json:"mime_type" binding:"required"`
	Size     int64      `json:"size"      binding:"required,gt=0"`
	Checksum string     `json:"checksum"  binding:"required,len=64,hexadecimal"`
	FolderID *uuid.UUID `json:"folder_id"`
}

type FileResponse struct {
	ID        uuid.UUID  `json:"id"`
	FolderID  *uuid.UUID `json:"folder_id"`
	FileName  string     `json:"file_name"`
	MimeType  string     `json:"mime_type"`
	Size      int64      `json:"size"`
	CreatedAt time.Time  `json:"created_at"`
}

type DownloadResponse struct {
	DownloadURL string `json:"download_url"`
}
