package repositories

import (
	"context"
	"errors"

	"github.com/AnkitSinha0/HashVault/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type StorageObjectRepository interface {
	Create(ctx context.Context, obj *models.StorageObject) error
	FindByChecksum(ctx context.Context, checksum string) (*models.StorageObject, error)
	// IncrementRefCount atomically adds 1 — called when a second user uploads
	// the same file content (deduplication hit).
	IncrementRefCount(ctx context.Context, id uuid.UUID) error
	// DecrementRefCount atomically subtracts 1 and returns the new count.
	// Caller deletes the S3 object when the returned count reaches 0.
	DecrementRefCount(ctx context.Context, id uuid.UUID) (int32, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type storageObjectRepo struct {
	db *gorm.DB
}

func NewStorageObjectRepository(db *gorm.DB) StorageObjectRepository {
	return &storageObjectRepo{db: db}
}

func (r *storageObjectRepo) Create(ctx context.Context, obj *models.StorageObject) error {
	return r.db.WithContext(ctx).Create(obj).Error
}

func (r *storageObjectRepo) FindByChecksum(ctx context.Context, checksum string) (*models.StorageObject, error) {
	var obj models.StorageObject
	err := r.db.WithContext(ctx).Where("checksum = ?", checksum).First(&obj).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &obj, err
}

func (r *storageObjectRepo) IncrementRefCount(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).
		Model(&models.StorageObject{}).
		Where("id = ?", id).
		UpdateColumn("ref_count", gorm.Expr("ref_count + 1")).
		Error
}

func (r *storageObjectRepo) DecrementRefCount(ctx context.Context, id uuid.UUID) (int32, error) {
	if err := r.db.WithContext(ctx).
		Model(&models.StorageObject{}).
		Where("id = ?", id).
		UpdateColumn("ref_count", gorm.Expr("ref_count - 1")).
		Error; err != nil {
		return 0, err
	}

	var obj models.StorageObject
	if err := r.db.WithContext(ctx).Select("ref_count").First(&obj, "id = ?", id).Error; err != nil {
		return 0, err
	}
	return obj.RefCount, nil
}

func (r *storageObjectRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&models.StorageObject{}, "id = ?", id).Error
}
