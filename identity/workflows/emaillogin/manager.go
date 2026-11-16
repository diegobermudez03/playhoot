// Package emaillogin is the one-time email code workflow: a Manager whose
// steps send a code to an address and take it back, signing the person in,
// registering them, or adding the address to an account, one file per step
// and every step of an attempt kept in its history.
package emaillogin

import (
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/business/accesstoken"
	"github.com/diegobermudez03/playhoot/identity/internal/business/refreshtoken"
	internalrepo "github.com/diegobermudez03/playhoot/identity/workflows/emaillogin/internal/repo"
	"github.com/diegobermudez03/playhoot/utils"
)

// accessIssuer issues access tokens.
type accessIssuer interface {
	Issue(user identity.User, issuedAt time.Time) (string, time.Time, error)
}

// Enqueuer leaves a task in Identity's queue inside a transaction.
type Enqueuer interface {
	Enqueue(tx *gorm.DB, taskType string, payload any, runAt time.Time, idempotencyKey string) error
}

// Manager is the email code workflow controller. Each step depends on its
// own narrow persistence contract, even though one internal/repo.Repo
// satisfies all of them.
type Manager struct {
	startRepo    startRepo
	completeRepo completeRepo
	linkRepo     linkRepo

	// dbServicer is the handle utils.RunInDBTransaction opens transactions
	// on; it is the same Repo, which owns the database handle.
	dbServicer utils.DBServicer

	queue      Enqueuer
	access     accessIssuer
	now        func() time.Time
	newRefresh func() (string, error)
}

// New constructs a Manager persisting to db, leaving the code's email in
// queue and signing access tokens with accessKey.
func New(db *gorm.DB, queue Enqueuer, accessKey []byte) (*Manager, error) {
	signer, err := accesstoken.NewSigner(accessKey)
	if err != nil {
		return nil, err
	}
	repo := internalrepo.New(db)
	return &Manager{
		startRepo:    repo,
		completeRepo: repo,
		linkRepo:     repo,
		dbServicer:   repo,
		queue:        queue,
		access:       signer,
		now:          time.Now,
		newRefresh:   refreshtoken.New,
	}, nil
}
