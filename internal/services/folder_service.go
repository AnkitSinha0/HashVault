package services

import (
	"context"

	"github.com/AnkitSinha0/HashVault/internal/dto"
	"github.com/AnkitSinha0/HashVault/internal/models"
	"github.com/AnkitSinha0/HashVault/internal/repositories"
	"github.com/google/uuid"
)

type FolderService interface {
	Create(ctx context.Context, userID uuid.UUID, req dto.CreateFolderRequest) (*dto.FolderResponse, error)
	Get(ctx context.Context, userID, folderID uuid.UUID) (*dto.FolderResponse, error)
	ListRoot(ctx context.Context, userID uuid.UUID) ([]dto.FolderResponse, error)
	ListChildren(ctx context.Context, userID, folderID uuid.UUID) ([]dto.FolderResponse, error)
	Rename(ctx context.Context, userID, folderID uuid.UUID, req dto.UpdateFolderRequest) (*dto.FolderResponse, error)
	Delete(ctx context.Context, userID, folderID uuid.UUID) error
}

type folderService struct {
	folders repositories.FolderRepository
}

func NewFolderService(folders repositories.FolderRepository) FolderService {
	return &folderService{folders: folders}
}

func (s *folderService) Create(ctx context.Context, userID uuid.UUID, req dto.CreateFolderRequest) (*dto.FolderResponse, error) {
	if req.ParentFolderID != nil {
		if _, err := s.folders.FindByID(ctx, *req.ParentFolderID, userID); err != nil {
			return nil, err
		}
	}

	folder := &models.Folder{
		UserID:         userID,
		ParentFolderID: req.ParentFolderID,
		Name:           req.Name,
	}
	if err := s.folders.Create(ctx, folder); err != nil {
		return nil, err
	}
	return toFolderResponse(folder), nil
}

func (s *folderService) Get(ctx context.Context, userID, folderID uuid.UUID) (*dto.FolderResponse, error) {
	folder, err := s.folders.FindByID(ctx, folderID, userID)
	if err != nil {
		return nil, err
	}
	return toFolderResponse(folder), nil
}

func (s *folderService) ListRoot(ctx context.Context, userID uuid.UUID) ([]dto.FolderResponse, error) {
	folders, err := s.folders.ListByParent(ctx, userID, nil)
	if err != nil {
		return nil, err
	}
	return toFolderResponses(folders), nil
}

func (s *folderService) ListChildren(ctx context.Context, userID, folderID uuid.UUID) ([]dto.FolderResponse, error) {
	// Confirm the parent folder belongs to this user before listing its children.
	if _, err := s.folders.FindByID(ctx, folderID, userID); err != nil {
		return nil, err
	}
	folders, err := s.folders.ListByParent(ctx, userID, &folderID)
	if err != nil {
		return nil, err
	}
	return toFolderResponses(folders), nil
}

func (s *folderService) Rename(ctx context.Context, userID, folderID uuid.UUID, req dto.UpdateFolderRequest) (*dto.FolderResponse, error) {
	folder, err := s.folders.FindByID(ctx, folderID, userID)
	if err != nil {
		return nil, err
	}
	folder.Name = req.Name
	if err := s.folders.Update(ctx, folder); err != nil {
		return nil, err
	}
	return toFolderResponse(folder), nil
}

func (s *folderService) Delete(ctx context.Context, userID, folderID uuid.UUID) error {
	return s.folders.Delete(ctx, folderID, userID)
}

func toFolderResponse(f *models.Folder) *dto.FolderResponse {
	return &dto.FolderResponse{
		ID:             f.ID,
		UserID:         f.UserID,
		ParentFolderID: f.ParentFolderID,
		Name:           f.Name,
		CreatedAt:      f.CreatedAt,
		UpdatedAt:      f.UpdatedAt,
	}
}

func toFolderResponses(folders []models.Folder) []dto.FolderResponse {
	out := make([]dto.FolderResponse, len(folders))
	for i := range folders {
		out[i] = *toFolderResponse(&folders[i])
	}
	return out
}

var _ FolderService = (*folderService)(nil)
