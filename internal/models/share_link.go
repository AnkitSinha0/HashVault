package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ShareLink struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey"          json:"id"`
	FileID    uuid.UUID  `gorm:"type:uuid;not null;index"       json:"file_id"`
	Token     string     `gorm:"uniqueIndex;not null;size:64"   json:"token"`
	ExpiresAt *time.Time `                                      json:"expires_at"`
	CreatedAt time.Time  `                                      json:"created_at"`
}

func (s *ShareLink) BeforeCreate(_ *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}
