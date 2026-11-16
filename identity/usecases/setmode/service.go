// Package setmode switches an account between its normal and its teaching
// face. Both are the same account: switching creates nothing and grants
// nothing, and what a mode may do is verified when it is done.
package setmode

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/business/accesstoken"
	internalrepo "github.com/diegobermudez03/playhoot/identity/usecases/setmode/internal/repo"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=setmode -destination=service_mock_test.go . accountStore,accessIssuer

// accountStore reads and changes an account's mode inside a transaction.
type accountStore interface {
	LockAccount(ctx context.Context, tx *gorm.DB, userUUID string) (*internalrepo.Account, error)
	ChangeMode(ctx context.Context, tx *gorm.DB, accountID int64, userID int64, fromMode string, mode string, changedAt time.Time) error
}

// accessIssuer issues access tokens.
type accessIssuer interface {
	Issue(user identity.User, issuedAt time.Time) (string, time.Time, error)
}

var (
	outcomeLabel = metrics.NewLabel("outcome", "switched", "unchanged",
		string(identity.ModeInvalid), string(identity.ModeNotAccount))
	modeLabel = metrics.NewLabel("mode", string(identity.ModeNormal), string(identity.ModeTeacher))

	switchesTotal = metrics.NewCounter(
		"identity_mode_switches_total",
		"Mode switches by the mode asked for and whether it changed, was already so, or was refused.",
		modeLabel, outcomeLabel,
	)
)

// Service is Identity's mode use case.
type Service struct {
	db       utils.DBServicer
	accounts accountStore
	access   accessIssuer
	now      func() time.Time
}

// New constructs a Service on db, signing access tokens with accessKey.
func New(db *gorm.DB, accessKey []byte) (*Service, error) {
	signer, err := accesstoken.NewSigner(accessKey)
	if err != nil {
		return nil, err
	}
	repo := internalrepo.New(db)
	return &Service{db: repo, accounts: repo, access: signer, now: time.Now}, nil
}

// switched is what the transaction decided: the User as they now are, and
// whether anything changed. An empty UUID means the outcome refuses.
type switched struct {
	user    identity.User
	outcome identity.SetModeOutcome
	metric  string
}

// SetMode puts the account of userUUID in mode and answers a fresh access
// token carrying it. identity.ErrUserNotFound when there is no such User.
func (s *Service) SetMode(ctx context.Context, userUUID string, mode string) (identity.SetModeResult, error) {
	if !identity.ValidMode(mode) {
		switchesTotal.Inc(ctx, modeLabel.Value(string(identity.ModeNormal)), outcomeLabel.Value(string(identity.ModeInvalid)))
		return identity.SetModeResult{Outcome: identity.ModeInvalid}, nil
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	decided, err := utils.RunInDBTransaction(ctx, s.db, func(ctx context.Context, tx *gorm.DB) (switched, error) {
		return s.setInTx(ctx, tx, userUUID, identity.Mode(mode), now)
	})
	if err != nil {
		return identity.SetModeResult{}, err
	}
	switchesTotal.Inc(ctx, modeLabel.Value(mode), outcomeLabel.Value(decided.metric))
	if decided.outcome != identity.ModeSet {
		return identity.SetModeResult{Outcome: decided.outcome}, nil
	}
	accessToken, expiresAt, err := s.access.Issue(decided.user, now)
	if err != nil {
		return identity.SetModeResult{}, err
	}
	return identity.SetModeResult{
		Outcome: identity.ModeSet,
		Credentials: identity.Credentials{
			User: decided.user, AccessToken: accessToken, AccessIssuedAt: now, AccessExpiresAt: expiresAt,
		},
	}, nil
}

func (s *Service) setInTx(ctx context.Context, tx *gorm.DB, userUUID string, mode identity.Mode, now time.Time) (switched, error) {
	account, err := s.accounts.LockAccount(ctx, tx, userUUID)
	if err != nil {
		return switched{}, err
	}
	if account == nil {
		return switched{}, identity.ErrUserNotFound
	}
	if account.AccountID == nil {
		return switched{outcome: identity.ModeNotAccount, metric: string(identity.ModeNotAccount)}, nil
	}
	current := identity.NormalizeMode(identity.Mode(account.Mode))
	user := identity.User{UUID: userUUID, DisplayName: account.DisplayName, Level: identity.LevelAccount, Mode: mode}
	if current == mode {
		return switched{user: user, outcome: identity.ModeSet, metric: "unchanged"}, nil
	}
	err = s.accounts.ChangeMode(ctx, tx, *account.AccountID, account.UserID, string(current), string(mode), now)
	if err != nil {
		return switched{}, err
	}
	return switched{user: user, outcome: identity.ModeSet, metric: "switched"}, nil
}
