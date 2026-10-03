// Package repo is sessionlifecycle's narrow persistence layer. It reports
// persisted facts and performs the mutations the Manager requests. It
// decides no business/lifecycle policy itself - that is decided by the
// Manager's own steps, never in here.
package repo

import "gorm.io/gorm"

// Repo is sessionlifecycle's persistence implementation. Its methods that
// mutate/read transaction-scoped state take an explicit tx *gorm.DB supplied
// by the Manager, which owns transaction scope; Repo never opens/commits its
// own transaction. Its unlocked, pre-transaction reads use the plain db handle instead.
type Repo struct {
	db *gorm.DB
}

// New constructs a Repo bound to db.
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// GetDB satisfies utils.DBServicer. Repo is the layer that actually owns the
// db handle, so it - not Manager - is the thing Manager passes to
// utils.RunInDBTransaction to open a transaction.
func (r *Repo) GetDB() *gorm.DB {
	return r.db
}
