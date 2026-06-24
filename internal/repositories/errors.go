package repositories

import "errors"
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

// ErrNotFound is returned when a queried record does not exist.
// Services check for this sentinel instead of importing gorm directly.
var ErrNotFound = errors.New("record not found")
