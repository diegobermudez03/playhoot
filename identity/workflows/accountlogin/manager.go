// Package accountlogin is the account login workflow: a Manager whose steps
// start a login, take the provider's answer and complete it, one file per
// step, keeping every step of an attempt in its history.
package accountlogin

import (
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/business/accesstoken"
	"github.com/diegobermudez03/playhoot/identity/internal/business/refreshtoken"
	"github.com/diegobermudez03/playhoot/identity/loginproviders"
	internalrepo "github.com/diegobermudez03/playhoot/identity/workflows/accountlogin/internal/repo"
	"github.com/diegobermudez03/playhoot/utils"
)

// providers are the enabled login providers.
type providers interface {
	Get(name identity.Provider) (loginproviders.Provider, bool)
}

// accessIssuer issues access tokens.
type accessIssuer interface {
	Issue(user identity.User, issuedAt time.Time) (string, time.Time, error)
}

// Manager is the account login workflow controller. Each step depends on its
// own narrow persistence contract, even though one internal/repo.Repo
// satisfies all of them.
type Manager struct {
	startRepo    startRepo
	callbackRepo callbackRepo
	completeRepo completeRepo

	// dbServicer is the handle utils.RunInDBTransaction opens transactions
	// on; it is the same Repo, which owns the database handle.
	dbServicer utils.DBServicer

	providers  providers
	access     accessIssuer
	now        func() time.Time
	newRefresh func() (string, error)
}

// New constructs a Manager persisting to db, with the enabled providers, and
// signing access tokens with accessKey.
func New(db *gorm.DB, registry *loginproviders.Registry, accessKey []byte) (*Manager, error) {
	signer, err := accesstoken.NewSigner(accessKey)
	if err != nil {
		return nil, err
	}
	repo := internalrepo.New(db)
	return &Manager{
		startRepo:    repo,
		callbackRepo: repo,
		completeRepo: repo,
		dbServicer:   repo,
		providers:    registry,
		access:       signer,
		now:          time.Now,
		newRefresh:   refreshtoken.New,
	}, nil
}
