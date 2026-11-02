// Package repo is versiondrafting's narrow persistence layer. It reports
// persisted facts and performs the mutations the Manager requests; it
// decides no business policy itself.
package repo

import "gorm.io/gorm"

// Repo is versiondrafting's persistence implementation. Every method takes
// the transaction the Manager opened; Repo never opens or commits one.
type Repo struct {
	db *gorm.DB
}

// New constructs a Repo bound to db.
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// GetDB satisfies utils.DBServicer, so the Manager opens its transactions on
// the handle Repo owns.
func (r *Repo) GetDB() *gorm.DB {
	return r.db
}
