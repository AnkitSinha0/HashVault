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
	return r.db.WithContext(ctx).Create(file).Error
}

func (r *fileRepo) FindByID(ctx context.Context, id, userID uuid.UUID) (*models.File, error) {
	var file models.File
	err := r.db.WithContext(ctx).
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
	q := r.db.WithContext(ctx).Where("user_id = ?", userID)
	if folderID == nil {
		q = q.Where("folder_id IS NULL")
	} else {
		q = q.Where("folder_id = ?", *folderID)
	}
	return files, q.Order("file_name ASC").Find(&files).Error
}

func (r *fileRepo) Delete(ctx context.Context, id, userID uuid.UUID) error {
	result := r.db.WithContext(ctx).
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
