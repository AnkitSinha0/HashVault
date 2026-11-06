package repositories

import (
	"context"
	"errors"

	"github.com/AnkitSinha0/HashVault/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type FileRepository interface {
	Create(ctx context.Context, file *models.File) error
	// FindByID fetches the file and its StorageObject in one query.
	// userID scoping prevents users from accessing each other's files.
	FindByID(ctx context.Context, id, userID uuid.UUID) (*models.File, error)
	ListByFolder(ctx context.Context, userID uuid.UUID, folderID *uuid.UUID) ([]models.File, error)
	Delete(ctx context.Context, id, userID uuid.UUID) error
	// UserHasLiveCopy reports whether userID owns a (not deleted) file whose
	// bytes have this checksum.
	UserHasLiveCopy(ctx context.Context, userID uuid.UUID, checksum string) (bool, error)
	// PurgeTombstones hard-deletes the soft-deleted file rows that still
	// point at a storage object, so the object row itself can be deleted.
	PurgeTombstones(ctx context.Context, storageObjectID uuid.UUID) error
}
// concrete implementation of interface
type fileRepo struct {
	db *gorm.DB
}
// constructor / factory 
func NewFileRepository(db *gorm.DB) FileRepository {
	return &fileRepo{db: db}
}

func (r *fileRepo) Create(ctx context.Context, file *models.File) error {
	return dbFromContext(ctx, r.db).WithContext(ctx).Create(file).Error
}

func (r *fileRepo) FindByID(ctx context.Context, id, userID uuid.UUID) (*models.File, error) {
	var file models.File
	err := dbFromContext(ctx, r.db).WithContext(ctx).
		Preload("StorageObject").
		Where("id = ? AND user_id = ?", id, userID).
		First(&file).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &file, err
}

func (r *fileRepo) ListByFolder(ctx context.Context, userID uuid.UUID, folderID *uuid.UUID) ([]models.File, error) {
	var files []models.File
	q := dbFromContext(ctx, r.db).WithContext(ctx).Where("user_id = ?", userID)
	if folderID == nil {
		q = q.Where("folder_id IS NULL")
	} else {
		q = q.Where("folder_id = ?", *folderID)
	}
	return files, q.Order("file_name ASC").Find(&files).Error
}

func (r *fileRepo) Delete(ctx context.Context, id, userID uuid.UUID) error {
	result := dbFromContext(ctx, r.db).WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		Delete(&models.File{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *fileRepo) UserHasLiveCopy(ctx context.Context, userID uuid.UUID, checksum string) (bool, error) {
	var n int64
	err := dbFromContext(ctx, r.db).WithContext(ctx).
		Model(&models.File{}).
		Joins("JOIN storage_objects ON storage_objects.id = files.storage_object_id").
		Where("files.user_id = ? AND storage_objects.checksum = ?", userID, checksum).
		Count(&n).Error
	return n > 0, err
}

// Unscoped turns off the automatic "deleted_at IS NULL" filter, and makes
// Delete a real DELETE instead of a soft delete. These rows can't be
// restored anyway: their bytes are about to be deleted.
func (r *fileRepo) PurgeTombstones(ctx context.Context, storageObjectID uuid.UUID) error {
	return dbFromContext(ctx, r.db).WithContext(ctx).
		Unscoped().
		Where("storage_object_id = ? AND deleted_at IS NOT NULL", storageObjectID).
		Delete(&models.File{}).
		Error
}
