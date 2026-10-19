package repositories

import (
	"context"

	"gorm.io/gorm"
)

type txKey struct{}

// TxManager runs fn inside a single DB transaction. Repository methods
// called with the ctx passed into fn automatically participate in that
// transaction — see dbFromContext. This keeps gorm out of the services
// package entirely: services call WithinTx and pass ctx down to the repo
// interfaces they already hold, without ever importing gorm themselves.
type TxManager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type gormTxManager struct {
	db *gorm.DB
}

func NewTxManager(db *gorm.DB) TxManager {
	return &gormTxManager{db: db}
}

func (m *gormTxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(context.WithValue(ctx, txKey{}, tx))
	})
}

// dbFromContext returns the transaction bound to ctx by WithinTx, or falls
// back to db (the repo's own handle) when called outside a transaction.
func dbFromContext(ctx context.Context, db *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(txKey{}).(*gorm.DB); ok {
		return tx
	}
	return db
}
