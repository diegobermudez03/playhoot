// Package repo persists the email code workflow: the attempts an address
// asked for, their append-only history, the accounts a completed code
// resolves to, and the sessions it opens.
package repo

import (
	"gorm.io/gorm"
)

// Repo is the email code workflow's persistence.
type Repo struct {
	db *gorm.DB
}

// New constructs a Repo on db.
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// GetDB returns the database, for transactions.
func (r *Repo) GetDB() *gorm.DB {
	return r.db
}
