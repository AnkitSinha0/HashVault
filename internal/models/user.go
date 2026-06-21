package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type User struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey"                json:"id"`
	Name         string         `gorm:"not null"                            json:"name"`
	Email        string         `gorm:"uniqueIndex;not null"                json:"email"`
	PasswordHash string         `gorm:"not null"                            json:"-"`
	StorageLimit int64          `gorm:"not null;default:10737418240"        json:"storage_limit"` // 10 GB
	UsedStorage  int64          `gorm:"not null;default:0"                  json:"used_storage"`
	CreatedAt    time.Time      `                                           json:"created_at"`
	UpdatedAt    time.Time      `                                           json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index"                               json:"-"`
}

func (u *User) BeforeCreate(_ *gorm.DB) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	return nil
}
