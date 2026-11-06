package services

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
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
	store   storage.ObjectStore
	tx      repositories.TxManager
}

func NewFileService(
	files repositories.FileRepository,
	objects repositories.StorageObjectRepository,
	folders repositories.FolderRepository,
	users repositories.UserRepository,
	store storage.ObjectStore,
	tx repositories.TxManager,
) FileService {
	return &fileService{files: files, objects: objects, folders: folders, users: users, store: store, tx: tx}
}

// InitUpload never hands out a URL for the shared objects/{sha256} key. The
// client uploads to its own staging key, and only ConfirmUpload, after
// checking the bytes, can put content under objects/.
func (s *fileService) InitUpload(ctx context.Context, userID uuid.UUID, req dto.InitUploadRequest) (*dto.InitUploadResponse, error) {
	checksum, err := normalizeChecksum(req.Checksum)
	if err != nil {
		return nil, err
	}
	if err := s.checkFolderOwnership(ctx, userID, req.FolderID); err != nil {
		return nil, err
	}
	// Advisory: the binding check happens atomically at confirm.
	if err := s.checkQuota(ctx, userID, req.Size); err != nil {
		return nil, err
	}

	skip, err := s.canSkipUpload(ctx, userID, checksum)
	if err != nil {
		return nil, err
	}
	if skip {
		return &dto.InitUploadResponse{Deduplicated: true}, nil
	}

	sum := checksumBase64(checksum)
	url, err := s.store.PresignPutURL(ctx, stagingKey(userID, checksum), req.Size, sum, uploadURLExpiry)
	if err != nil {
		return nil, err
	}
	return &dto.InitUploadResponse{
		UploadURL:     url,
		UploadHeaders: map[string]string{"x-amz-checksum-sha256": sum},
	}, nil
}

func (s *fileService) ConfirmUpload(ctx context.Context, userID uuid.UUID, req dto.ConfirmUploadRequest) (*dto.FileResponse, error) {
	checksum, err := normalizeChecksum(req.Checksum)
	if err != nil {
		return nil, err
	}
	if err := s.checkFolderOwnership(ctx, userID, req.FolderID); err != nil {
		return nil, err
	}

	skip, err := s.canSkipUpload(ctx, userID, checksum)
	if err != nil {
		return nil, err
	}
	// Unless the upload could be skipped, the caller must have uploaded bytes
	// that the store itself confirms hash to the checksum.
	var staged *storage.ObjectInfo
	if !skip {
		if staged, err = s.verifyStaged(ctx, userID, checksum, req.Size); err != nil {
			return nil, err
		}
	}

	var file *models.File
	var size int64
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		obj, inserted, err := s.objects.Upsert(ctx, checksum, contentKey(checksum), req.Size)
		if err != nil {
			return err
		}
		if inserted {
			// New content. The row is locked until commit, so nobody else can
			// reference it before the bytes are in place.
			if staged == nil {
				// The object we planned to reuse was deleted in the meantime.
				return ErrUploadNotVerified
			}
			if err := s.store.CopyObject(ctx, stagingKey(userID, checksum), obj.S3Key, staged.ETag); err != nil {
				return err
			}
		}

		// Charge the object's real size, the same number Delete refunds.
		if ok, err := s.users.ReserveStorage(ctx, userID, obj.Size); err != nil {
			return err
		} else if !ok {
			return ErrStorageExceeded
		}

		file = &models.File{
			UserID:          userID,
			FolderID:        req.FolderID,
			StorageObjectID: obj.ID,
			FileName:        req.FileName,
			MimeType:        req.MimeType,
		}
		size = obj.Size
		return s.files.Create(ctx, file)
	})
	if err != nil {
		return nil, err
	}

	if staged != nil {
		// Best effort: the lifecycle rule on staging/ removes it anyway.
		_ = s.store.DeleteObject(ctx, stagingKey(userID, checksum))
	}

	return &dto.FileResponse{
		ID:        file.ID,
		FolderID:  file.FolderID,
		FileName:  file.FileName,
		MimeType:  file.MimeType,
		Size:      size,
		CreatedAt: file.CreatedAt,
	}, nil
}

func (s *fileService) Download(ctx context.Context, userID, fileID uuid.UUID) (*dto.DownloadResponse, error) {
	file, err := s.files.FindByID(ctx, fileID, userID)
	if err != nil {
		return nil, err
	}
	url, err := s.store.PresignGetURL(ctx, file.StorageObject.S3Key, file.FileName, downloadURLExpiry)
	if err != nil {
		return nil, err
	}
	return &dto.DownloadResponse{DownloadURL: url}, nil
}

// Delete is one transaction: either every step happens or none does.
//
// The old version ran each step on its own and deleted the S3 bytes before
// the storage_objects row. The row delete then failed (the soft-deleted file
// row still references it through fk_files_storage_object), leaving a
// ref_count-0 row whose bytes were gone, and every later upload of the same
// content was linked to that dead row.
func (s *fileService) Delete(ctx context.Context, userID, fileID uuid.UUID) error {
	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		file, err := s.files.FindByID(ctx, fileID, userID)
		if err != nil {
			return err
		}
		// Scoped soft delete. If two requests delete the same file at once,
		// the second one matches no row here and gets ErrNotFound.
		if err := s.files.Delete(ctx, fileID, userID); err != nil {
			return err
		}
		if err := s.users.IncrementStorage(ctx, userID, -file.StorageObject.Size); err != nil {
			return err
		}

		// Locks the object's row until commit. A concurrent confirm for the
		// same content waits here, so it can't reuse bytes we are deleting.
		left, err := s.objects.DecrementRefCount(ctx, file.StorageObjectID)
		if err != nil {
			return err
		}
		if left > 0 {
			return nil
		}

		// Last reference: remove the soft-deleted file rows that still point
		// at the object, then the object row, then the bytes. The bytes go
		// last so that if deleting them fails, the transaction rolls back and
		// nothing has changed. The client can simply retry.
		if err := s.files.PurgeTombstones(ctx, file.StorageObjectID); err != nil {
			return err
		}
		if err := s.objects.Delete(ctx, file.StorageObjectID); err != nil {
			return err
		}
		return s.store.DeleteObject(ctx, file.StorageObject.S3Key)
	})
}

// canSkipUpload decides whether the client may confirm without uploading.
// Only when it already owns a live file with these bytes: knowing a hash
// proves nothing, but owning a file with it proves you had the bytes.
// Everyone else uploads, and storage is still shared once the bytes are
// verified (the upsert adds a reference instead of a second copy). It also
// means init-upload no longer tells anyone whether *other* users store a file.
func (s *fileService) canSkipUpload(ctx context.Context, userID uuid.UUID, checksum string) (bool, error) {
	return s.files.UserHasLiveCopy(ctx, userID, checksum)
}

// verifyStaged checks the uploaded bytes using only what the store recorded.
// An upload sent without the checksum header has no recorded SHA-256 and
// fails here, so leaving the header off doesn't get around the check.
func (s *fileService) verifyStaged(ctx context.Context, userID uuid.UUID, checksum string, size int64) (*storage.ObjectInfo, error) {
	info, err := s.store.StatObject(ctx, stagingKey(userID, checksum))
	if errors.Is(err, storage.ErrObjectNotFound) {
		return nil, ErrUploadNotVerified
	}
	if err != nil {
		return nil, err
	}
	if info.SHA256 != checksumBase64(checksum) || info.Size != size {
		return nil, ErrUploadNotVerified
	}
	return info, nil
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

var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// normalizeChecksum lowercases the hash, so "ABC…" and "abc…" name the same
// object, and rejects anything that isn't exactly 64 hex digits. (The DTO's
// "hexadecimal" validator also accepts a "0x" prefix.)
func normalizeChecksum(c string) (string, error) {
	c = strings.ToLower(c)
	if !sha256Hex.MatchString(c) {
		return "", ErrInvalidChecksum
	}
	return c, nil
}

// checksumBase64 converts the hex SHA-256 to the base64 form S3 uses.
func checksumBase64(hexSum string) string {
	raw, _ := hex.DecodeString(hexSum) // validated by normalizeChecksum
	return base64.StdEncoding.EncodeToString(raw)
}

// contentKey is content-addressable: identical content always maps to the
// same key. Only ConfirmUpload writes here, and only verified bytes.
func contentKey(checksum string) string {
	return fmt.Sprintf("objects/%s", checksum)
}

// stagingKey is private to one user, so one user's upload can never touch
// another user's bytes.
func stagingKey(userID uuid.UUID, checksum string) string {
	return fmt.Sprintf("%s%s/%s", storage.StagingPrefix, userID, checksum)
}

var _ FileService = (*fileService)(nil)
