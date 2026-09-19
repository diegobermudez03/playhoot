// Package repo is sessionlifecycle's narrow persistence layer. It reports
// persisted facts (locked row state, existing Actor/Participant/JoinCode
// rows, existing idempotency claims) and performs the mutations the Manager
// requests. It decides no business/lifecycle policy itself (admission,
// expiration, idempotency-replay meaning) - see
// `docs/engineering/standards/domain-logic-placement.md`'s Responsibility
// Categories and `docs/engineering/standards/repositories.md`'s Repository
// Naming section.
package repo

import "gorm.io/gorm"

// Repo is sessionlifecycle's persistence implementation. Its methods that
// mutate/read transaction-scoped state take an explicit tx *gorm.DB supplied
// by the Manager, which owns transaction scope (see
// `docs/engineering/standards/repositories.md`'s Transaction Ownership
// section); Repo never opens/commits its own transaction. Its unlocked,
// pre-transaction reads (for example resolving a JoinCode before the Game
// Management read that must happen before the mutation transaction opens,
// per GAME-ADR-0001) use the plain db handle instead.
type Repo struct {
	db *gorm.DB
}

// New constructs a Repo bound to db.
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}
