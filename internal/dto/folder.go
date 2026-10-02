package dto

import (
	"time"

	"github.com/google/uuid"
)

type CreateFolderRequest struct {
	Name           string     `json:"name"             binding:"required,min=1,max=255"`
	ParentFolderID *uuid.UUID `json:"parent_folder_id"`
}

type UpdateFolderRequest struct {
	Name string `json:"name" binding:"required,min=1,max=255"`
}

type FolderResponse struct {
	ID             uuid.UUID  `json:"id"`
	UserID         uuid.UUID  `json:"user_id"`
	ParentFolderID *uuid.UUID `json:"parent_folder_id"`
	Name           string     `json:"name"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}


