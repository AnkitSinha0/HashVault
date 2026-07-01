package repositories

import (
	"context"
	"errors"

	"github.com/AnkitSinha0/HashVault/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ShareLinkRepository interface {
	Create(ctx context.Context, link *models.ShareLink) error
	FindByToken(ctx context.Context, token string) (*models.ShareLink, error)
	ListByFileID(ctx context.Context, fileID uuid.UUID) ([]models.ShareLink, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type shareLinkRepo struct {
	db *gorm.DB
}

func NewShareLinkRepository(db *gorm.DB) ShareLinkRepository {
	return &shareLinkRepo{db: db}
}

func (r *shareLinkRepo) Create(ctx context.Context, link *models.ShareLink) error {
	return r.db.WithContext(ctx).Create(link).Error
}

func (r *shareLinkRepo) FindByToken(ctx context.Context, token string) (*models.ShareLink, error) {
	var link models.ShareLink
	err := r.db.WithContext(ctx).Where("token = ?", token).First(&link).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &link, err
}

func (r *shareLinkRepo) ListByFileID(ctx context.Context, fileID uuid.UUID) ([]models.ShareLink, error) {
	var links []models.ShareLink
	return links, r.db.WithContext(ctx).Where("file_id = ?", fileID).Find(&links).Error
}

func (r *shareLinkRepo) Delete(ctx context.Context, id uuid.UUID) error {
	result := r.db.WithContext(ctx).Delete(&models.ShareLink{}, "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
