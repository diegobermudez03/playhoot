// Package repo is gamelifecycle's narrow persistence layer. It reports
// persisted facts and performs the mutations the Manager requests; it decides
// no lifecycle policy itself.
package repo

import "gorm.io/gorm"

// Repo is gamelifecycle's persistence implementation. Its methods take the
// transaction the Manager opened; it never opens or commits one itself.
type Repo struct {
	db *gorm.DB
}

// New constructs a Repo bound to db.
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// GetDB satisfies utils.DBServicer, so the Manager opens transactions on the
// handle this Repo owns.
func (r *Repo) GetDB() *gorm.DB {
	return r.db
}
