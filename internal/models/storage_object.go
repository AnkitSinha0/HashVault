package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// StorageObject is one physical file on S3.
// Many File records can reference the same StorageObject — deduplication via SHA-256.
// RefCount tracks how many File rows point here; when it hits 0 the S3 object is deleted.
type StorageObject struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey"          json:"id"`
	Checksum  string    `gorm:"uniqueIndex;not null;size:64"   json:"checksum"` // SHA-256 hex
	S3Key     string    `gorm:"not null"                       json:"-"`
	Size      int64     `gorm:"not null"                       json:"size"`
	RefCount  int32     `gorm:"not null;default:1"             json:"-"`
	CreatedAt time.Time `                                      json:"created_at"`
}

func (o *StorageObject) BeforeCreate(_ *gorm.DB) error {
	if o.ID == uuid.Nil {
		o.ID = uuid.New()
	}
	return nil
}
