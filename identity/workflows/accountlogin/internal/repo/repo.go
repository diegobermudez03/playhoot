// Package repo persists the account login workflow: its attempts and their
// history of events, the accounts external identities resolve to, and the
// sessions a completed login opens.
package repo

import (
	"gorm.io/gorm"
)

// Repo is the account login workflow's persistence.
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
