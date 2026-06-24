package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)
// *uuid because folder is optional
// folder /file somtimes needs to means : there is no parent folder or this file is not
// inside any folder

// even uuid.Nil means 00000..
// pointer solves this nil measn in SQL NULL
// or some value so folder has parent
type Folder struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey"                    json:"id"`
	UserID         uuid.UUID      `gorm:"type:uuid;not null;index:idx_folder_user" json:"user_id"`
	ParentFolderID *uuid.UUID     `gorm:"type:uuid;index"                          json:"parent_folder_id"`
	Name           string         `gorm:"not null"                                 json:"name"`
	CreatedAt      time.Time      `                                                json:"created_at"`
	UpdatedAt      time.Time      `                                                json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index"                                    json:"-"`
}

func (f *Folder) BeforeCreate(_ *gorm.DB) error {
	if f.ID == uuid.Nil {
		f.ID = uuid.New()
	}
	return nil
}
