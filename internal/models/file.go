package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type File struct {
	ID              uuid.UUID      `gorm:"type:uuid;primaryKey"                          json:"id"`
	UserID          uuid.UUID      `gorm:"type:uuid;not null;index:idx_files_user_folder" json:"user_id"`
	FolderID        *uuid.UUID     `gorm:"type:uuid;index:idx_files_user_folder"          json:"folder_id"`
	StorageObjectID uuid.UUID      `gorm:"type:uuid;not null"                             json:"-"`
	FileName        string         `gorm:"not null"                                       json:"file_name"`
	MimeType        string         `gorm:"not null"                                       json:"mime_type"`
	CreatedAt       time.Time      `                                                      json:"created_at"`
	UpdatedAt       time.Time      `                                                      json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index"                                          json:"-"`

	StorageObject StorageObject `gorm:"foreignKey:StorageObjectID" json:"-"`
}

func (f *File) BeforeCreate(_ *gorm.DB) error {
	if f.ID == uuid.Nil {
		f.ID = uuid.New()
	}
	return nil
}
