package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/AnkitSinha0/HashVault/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type StorageObjectRepository interface {
	FindByChecksum(ctx context.Context, checksum string) (*models.StorageObject, error)
	// Upsert adds a reference to the object with this checksum, creating it
	// (ref_count 1) if it doesn't exist, in one statement. inserted reports
	// whether the row is new. The row stays locked until the transaction ends.
	Upsert(ctx context.Context, checksum, s3Key string, size int64) (obj *models.StorageObject, inserted bool, err error)
	// DecrementRefCount subtracts 1 and returns the new count, in one
	// statement. The row stays locked until the transaction ends.
	DecrementRefCount(ctx context.Context, id uuid.UUID) (int32, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type storageObjectRepo struct {
	db *gorm.DB
}

func NewStorageObjectRepository(db *gorm.DB) StorageObjectRepository {
	return &storageObjectRepo{db: db}
}

func (r *storageObjectRepo) FindByChecksum(ctx context.Context, checksum string) (*models.StorageObject, error) {
	var obj models.StorageObject
	err := dbFromContext(ctx, r.db).WithContext(ctx).Where("checksum = ?", checksum).First(&obj).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &obj, err
}

type upsertRow struct {
	ID        uuid.UUID
	Checksum  string
	S3Key     string
	Size      int64
	RefCount  int32
	CreatedAt time.Time
	Inserted  bool
}

// Two first uploads of the same bytes used to both see "not found" and both
// INSERT; the loser hit the unique index and got a 500. ON CONFLICT turns the
// loser's INSERT into the UPDATE. xmax is 0 only on a row this statement
// inserted, which is how the caller learns whether it must store the bytes.
func (r *storageObjectRepo) Upsert(ctx context.Context, checksum, s3Key string, size int64) (*models.StorageObject, bool, error) {
	var row upsertRow
	err := dbFromContext(ctx, r.db).WithContext(ctx).Raw(`
		INSERT INTO storage_objects (id, checksum, s3_key, size, ref_count, created_at)
		VALUES (?, ?, ?, ?, 1, NOW())
		ON CONFLICT (checksum) DO UPDATE SET ref_count = storage_objects.ref_count + 1
		RETURNING id, checksum, s3_key, size, ref_count, created_at, (xmax = 0) AS inserted`,
		uuid.New(), checksum, s3Key, size,
	).Scan(&row).Error
	if err != nil {
		return nil, false, err
	}
	return &models.StorageObject{
		ID:        row.ID,
		Checksum:  row.Checksum,
		S3Key:     row.S3Key,
		Size:      row.Size,
		RefCount:  row.RefCount,
		CreatedAt: row.CreatedAt,
	}, row.Inserted, nil
}

// RETURNING hands back the value this UPDATE produced. The old version read it
// with a separate SELECT, so two concurrent deletes could both read 0.
func (r *storageObjectRepo) DecrementRefCount(ctx context.Context, id uuid.UUID) (int32, error) {
	var left int32
	result := dbFromContext(ctx, r.db).WithContext(ctx).Raw(
		`UPDATE storage_objects SET ref_count = ref_count - 1 WHERE id = ? RETURNING ref_count`, id,
	).Scan(&left)
	if result.Error != nil {
		return 0, result.Error
	}
	if result.RowsAffected == 0 {
		return 0, ErrNotFound
	}
	return left, nil
}

func (r *storageObjectRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return dbFromContext(ctx, r.db).WithContext(ctx).Delete(&models.StorageObject{}, "id = ?", id).Error
}
