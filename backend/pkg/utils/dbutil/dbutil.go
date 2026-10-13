// Source: getarcaneapp/arcane/backend/pkg/utils/dbutil/dbutil.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: functional cut: removed SQLite runtime and migrations.

// Package dbutil provides small generic helpers around GORM that consolidate
// repetitive single-row lookup and transaction boilerplate found across services.
package dbutil

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// IsRetryableWriteError reports whether a database write failed because the
// database was temporarily busy or timed out.
func IsRetryableWriteError(err error) bool {
	if err == nil {
		return false
	}

	var pgError *pgconn.PgError
	if errors.As(err, &pgError) {
		switch pgError.Code {
		case "40001", "40P01", "55P03":
			return true
		}
	}

	errMsg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(errMsg, "timeout")
}

// FirstWhere fetches a single row of T matching the provided WHERE clause.
// It maps gorm.ErrRecordNotFound to notFound (when non-nil) and otherwise wraps
// the error with a generic "failed to query <T>" prefix.
//
// Example:
//
//	user, err := dbutil.FirstWhere[user.User](ctx, s.db.DB, ErrUserNotFound, "username = ?", username)
func FirstWhere[T any](ctx context.Context, db *gorm.DB, notFound error, where string, args ...any) (*T, error) {
	var out T
	if err := db.WithContext(ctx).Where(where, args...).First(&out).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) && notFound != nil {
			return nil, notFound
		}
		return nil, fmt.Errorf("failed to query %T: %w", out, err)
	}
	return &out, nil
}

// WithTx runs fn inside a GORM transaction bound to ctx. Equivalent to
// db.WithContext(ctx).Transaction(fn) but tightens the surface area at call sites.
func WithTx(ctx context.Context, db *gorm.DB, fn func(tx *gorm.DB) error) error {
	return db.WithContext(ctx).Transaction(fn)
}
