package services

import (
	"context"
	"fmt"
	"time"

	"github.com/AnkitSinha0/HashVault/internal/dto"
	"github.com/AnkitSinha0/HashVault/internal/models"
	"github.com/AnkitSinha0/HashVault/internal/repositories"
	"github.com/AnkitSinha0/HashVault/internal/storage"
	"github.com/google/uuid"
)

const (
	uploadURLExpiry   = 15 * time.Minute
	downloadURLExpiry = 15 * time.Minute
)

type FileService interface {
	InitUpload(ctx context.Context, userID uuid.UUID, req dto.InitUploadRequest) (*dto.InitUploadResponse, error)
	ConfirmUpload(ctx context.Context, userID uuid.UUID, req dto.ConfirmUploadRequest) (*dto.FileResponse, error)
	Download(ctx context.Context, userID, fileID uuid.UUID) (*dto.DownloadResponse, error)
	Delete(ctx context.Context, userID, fileID uuid.UUID) error
}

type fileService struct {
	files   repositories.FileRepository
	objects repositories.StorageObjectRepository
	folders repositories.FolderRepository
	users   repositories.UserRepository
	s3      *storage.S3Client
}

func NewFileService(
	files repositories.FileRepository,
	objects repositories.StorageObjectRepository,
	folders repositories.FolderRepository,
	users repositories.UserRepository,
	s3 *storage.S3Client,
) FileService {
	return &fileService{files: files, objects: objects, folders: folders, users: users, s3: s3}
}

func (s *fileService) InitUpload(ctx context.Context, userID uuid.UUID, req dto.InitUploadRequest) (*dto.InitUploadResponse, error) {
	if err := s.checkFolderOwnership(ctx, userID, req.FolderID); err != nil {
		return nil, err
	}
	if err := s.checkQuota(ctx, userID, req.Size); err != nil {
		return nil, err
	}

	_, err := s.objects.FindByChecksum(ctx, req.Checksum)
	if err == nil {
		// Content already stored — client skips the upload entirely.
		return &dto.InitUploadResponse{Deduplicated: true}, nil
	}
	if err != repositories.ErrNotFound {
		return nil, err
	}

	key := storageKey(req.Checksum)
	url, err := s.s3.PresignPutURL(ctx, key, uploadURLExpiry)
	if err != nil {
		return nil, err
	}
	return &dto.InitUploadResponse{UploadURL: url, Deduplicated: false}, nil
}

func (s *fileService) ConfirmUpload(ctx context.Context, userID uuid.UUID, req dto.ConfirmUploadRequest) (*dto.FileResponse, error) {
	if err := s.checkFolderOwnership(ctx, userID, req.FolderID); err != nil {
		return nil, err
	}
	if err := s.checkQuota(ctx, userID, req.Size); err != nil {
		return nil, err
	}

	obj, err := s.objects.FindByChecksum(ctx, req.Checksum)
	switch {
	case err == nil:
		if incErr := s.objects.IncrementRefCount(ctx, obj.ID); incErr != nil {
			return nil, incErr
		}
	case err == repositories.ErrNotFound:
		obj = &models.StorageObject{
			Checksum: req.Checksum,
			S3Key:    storageKey(req.Checksum),
			Size:     req.Size,
		}
		if createErr := s.objects.Create(ctx, obj); createErr != nil {
			return nil, createErr
		}
	default:
		return nil, err
	}

	file := &models.File{
		UserID:          userID,
		FolderID:        req.FolderID,
		StorageObjectID: obj.ID,
		FileName:        req.FileName,
		MimeType:        req.MimeType,
	}
	if err := s.files.Create(ctx, file); err != nil {
		return nil, err
	}

	if err := s.users.IncrementStorage(ctx, userID, req.Size); err != nil {
		return nil, err
	}

	return &dto.FileResponse{
		ID:        file.ID,
		FolderID:  file.FolderID,
		FileName:  file.FileName,
		MimeType:  file.MimeType,
		Size:      obj.Size,
		CreatedAt: file.CreatedAt,
	}, nil
}

func (s *fileService) Download(ctx context.Context, userID, fileID uuid.UUID) (*dto.DownloadResponse, error) {
	file, err := s.files.FindByID(ctx, fileID, userID)
	if err != nil {
		return nil, err
	}
	url, err := s.s3.PresignGetURL(ctx, file.StorageObject.S3Key, downloadURLExpiry)
	if err != nil {
		return nil, err
	}
	return &dto.DownloadResponse{DownloadURL: url}, nil
}

func (s *fileService) Delete(ctx context.Context, userID, fileID uuid.UUID) error {
	file, err := s.files.FindByID(ctx, fileID, userID)
	if err != nil {
		return err
	}

	if err := s.files.Delete(ctx, fileID, userID); err != nil {
		return err
	}

	newCount, err := s.objects.DecrementRefCount(ctx, file.StorageObjectID)
	if err != nil {
		return err
	}
	if newCount <= 0 {
		if err := s.s3.DeleteObject(ctx, file.StorageObject.S3Key); err != nil {
			return err
		}
		if err := s.objects.Delete(ctx, file.StorageObjectID); err != nil {
			return err
		}
	}

	return s.users.IncrementStorage(ctx, userID, -file.StorageObject.Size)
}

func (s *fileService) checkFolderOwnership(ctx context.Context, userID uuid.UUID, folderID *uuid.UUID) error {
	if folderID == nil {
		return nil
	}
	_, err := s.folders.FindByID(ctx, *folderID, userID)
	return err
}

func (s *fileService) checkQuota(ctx context.Context, userID uuid.UUID, size int64) error {
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if user.UsedStorage+size > user.StorageLimit {
		return ErrStorageExceeded
	}
	return nil
}

// storageKey is content-addressable: identical content always maps to the
// same S3 key, so a second upload of the same bytes never re-uploads.
func storageKey(checksum string) string {
	return fmt.Sprintf("objects/%s", checksum)
}

var _ FileService = (*fileService)(nil)
