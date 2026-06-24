package repositories

import (
	"context"
	"errors"

	"github.com/AnkitSinha0/HashVault/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UserRepository interface {
	Create(ctx context.Context, user *models.User) error
	FindByEmail(ctx context.Context, email string) (*models.User, error)
	FindByID(ctx context.Context, id uuid.UUID) (*models.User, error)
	// IncrementStorage adds delta bytes to used_storage atomically.
	// delta is negative when freeing space. '_____'
	IncrementStorage(ctx context.Context, id uuid.UUID, delta int64) error
}

type userRepo struct {
	db *gorm.DB
}
//constuctor (constructs the ready to use object) :)
func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepo{db: db}
}

func (r *userRepo) Create(ctx context.Context, user *models.User) error {
	return r.db.WithContext(ctx).Create(user).Error
}

func (r *userRepo) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	var user models.User
	err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &user, err
}
//*models.user is a pointer and pointers can be nil so thats why we have 
// return value as *models.user not models.User cause struct can't be nil 
// and we chose to have return as nil or the actual user object
func (r *userRepo) FindByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	var user models.User
	err := r.db.WithContext(ctx).First(&user, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &user, err
}

func (r *userRepo) IncrementStorage(ctx context.Context, id uuid.UUID, delta int64) error {
	return r.db.WithContext(ctx).
		Model(&models.User{}).
		Where("id = ?", id).
		UpdateColumn("used_storage", gorm.Expr("used_storage + ?", delta)).
		Error
}
