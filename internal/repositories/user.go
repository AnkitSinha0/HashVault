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

// why &user not user because we creatd the user object and we are giving gorm 
// the address of that object to be filled 
// var ErrNotFound = errors.New("not found")
// why ErrNotFound ; cause we dont want to leak gorm specific repositry level error
// to our service layer 
// “This was not a normal not-found case. Something else actually went wrong with the DB/query.”
// So repo just passes the error upward.
// Problem: if we returned error
//
// service now knows about GORM
// service is coupled to DB library details
// if you later switch from GORM to raw SQL / sqlx / pgx, service code may need changes

func (r *userRepo) FindByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	var user models.User
	err := r.db.WithContext(ctx).First(&user, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &user, err
}
// Model(&models.user{}) 
// Important: this does not mean we are updating this specific Go object.
// This empty struct is just used to tell GORM which model/table to target.
// So it’s basically:
// use the users table



// IMPORTANT WHY NOT FETCH AND MODIFY ?
// BECAUSE 2 DB OPERATION
// SAFER FOR CONCURRENT OPERATION

// WHY updatecolumn not update cause update will trigger updatedAt etc
func (r *userRepo) IncrementStorage(ctx context.Context, id uuid.UUID, delta int64) error {
	return r.db.WithContext(ctx).
		Model(&models.User{}).
		Where("id = ?", id).
		UpdateColumn("used_storage", gorm.Expr("used_storage + ?", delta)).
		Error
}
