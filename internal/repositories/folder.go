package repositories

import (
	"context"
	"errors"

	"github.com/AnkitSinha0/HashVault/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type FolderRepository interface {
	Create(ctx context.Context, folder *models.Folder) error
	FindByID(ctx context.Context, id, userID uuid.UUID) (*models.Folder, error)
	// ListByParent returns all folders under parentID for the given user.
	// Pass nil parentID to list root-level folders.
	ListByParent(ctx context.Context, userID uuid.UUID, parentID *uuid.UUID) ([]models.Folder, error)
	Update(ctx context.Context, folder *models.Folder) error
	Delete(ctx context.Context, id, userID uuid.UUID) error
}

type folderRepo struct {
	db *gorm.DB
}

func NewFolderRepository(db *gorm.DB) FolderRepository {
	return &folderRepo{db: db}
}

func (r *folderRepo) Create(ctx context.Context, folder *models.Folder) error {
	return r.db.WithContext(ctx).Create(folder).Error
}

func (r *folderRepo) FindByID(ctx context.Context, id, userID uuid.UUID) (*models.Folder, error) {
	var folder models.Folder
	err := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		First(&folder).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &folder, err
}

func (r *folderRepo) ListByParent(ctx context.Context, userID uuid.UUID, parentID *uuid.UUID) ([]models.Folder, error) {
	var folders []models.Folder
	q := r.db.WithContext(ctx).Where("user_id = ?", userID)
	if parentID == nil {
		q = q.Where("parent_folder_id IS NULL")
	} else {
		q = q.Where("parent_folder_id = ?", *parentID)
	}
	return folders, q.Order("name ASC").Find(&folders).Error
}

func (r *folderRepo) Update(ctx context.Context, folder *models.Folder) error {
	return r.db.WithContext(ctx).Save(folder).Error
}

func (r *folderRepo) Delete(ctx context.Context, id, userID uuid.UUID) error {
	result := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		Delete(&models.Folder{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
