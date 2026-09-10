// Package pgerrs recognizes specific PostgreSQL error conditions that
// Session Runtime's repository layer treats as expected outcomes rather than
// infrastructure failures - most notably a unique-constraint violation,
// which both the idempotency claim mechanism and JoinCode generation rely on
// to detect a collision rather than a generic error.
package pgerrs

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

const uniqueViolationCode = "23505"

// IsUniqueViolation reports whether err is a PostgreSQL unique-constraint
// violation (SQLSTATE 23505).
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == uniqueViolationCode
	}
	return false
}
