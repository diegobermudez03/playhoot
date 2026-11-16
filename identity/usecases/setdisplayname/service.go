// Package setdisplayname names a User: the name other people see for them.
// An account has none until its person chooses one; every change is kept.
package setdisplayname

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/business/displayname"
	internalrepo "github.com/diegobermudez03/playhoot/identity/usecases/setdisplayname/internal/repo"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=setdisplayname -destination=service_mock_test.go . userStore

// userStore persists display names.
type userStore interface {
	LockUser(ctx context.Context, tx *gorm.DB, userUUID string) (*internalrepo.User, error)
	ChangesSince(ctx context.Context, tx *gorm.DB, userID int64, since time.Time) ([]time.Time, error)
	ChangeDisplayName(ctx context.Context, tx *gorm.DB, userID int64, fromName *string, displayName string, changedAt time.Time) error
}

var (
	outcomeLabel = metrics.NewLabel("outcome", "first", "changed", "unchanged",
		string(identity.DisplayNameInvalid), string(identity.DisplayNameLimitReached))

	changesTotal = metrics.NewCounter(
		"identity_display_name_changes_total",
		"Display names chosen, by whether it was the first one, a change, the same name, or refused.",
		outcomeLabel,
	)
)

// Service is Identity's display name use case.
type Service struct {
	db    utils.DBServicer
	users userStore
	now   func() time.Time
}

// New constructs a Service on db.
func New(db *gorm.DB) *Service {
	repo := internalrepo.New(db)
	return &Service{db: repo, users: repo, now: time.Now}
}

// SetDisplayName names the User with userUUID. A User may change their name
// a few times per rolling window; beyond that the refusal says when the next
// change is possible. identity.ErrUserNotFound when there is no such User.
func (s *Service) SetDisplayName(ctx context.Context, userUUID string, displayName string) (identity.SetDisplayNameResult, error) {
	name, ok := displayname.Normalize(displayName)
	if !ok {
		changesTotal.Inc(ctx, outcomeLabel.Value(string(identity.DisplayNameInvalid)))
		return identity.SetDisplayNameResult{Outcome: identity.DisplayNameInvalid}, nil
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	return utils.RunInDBTransaction(ctx, s.db, func(ctx context.Context, tx *gorm.DB) (identity.SetDisplayNameResult, error) {
		return s.setInTx(ctx, tx, userUUID, name, now)
	})
}

func (s *Service) setInTx(ctx context.Context, tx *gorm.DB, userUUID string, name string, now time.Time) (identity.SetDisplayNameResult, error) {
	user, err := s.users.LockUser(ctx, tx, userUUID)
	if err != nil {
		return identity.SetDisplayNameResult{}, err
	}
	if user == nil {
		return identity.SetDisplayNameResult{}, identity.ErrUserNotFound
	}
	level := identity.LevelGuest
	if user.HasAccount {
		level = identity.LevelAccount
	}
	named := identity.SetDisplayNameResult{
		Outcome: identity.DisplayNameSet,
		User:    identity.User{UUID: userUUID, DisplayName: name, Level: level, Mode: identity.NormalizeMode(identity.Mode(user.Mode))},
	}
	outcome := "changed"
	switch {
	case user.DisplayName == nil:
		outcome = "first"
	case *user.DisplayName == name:
		changesTotal.Inc(ctx, outcomeLabel.Value("unchanged"))
		return named, nil
	}
	inWindow, err := s.users.ChangesSince(ctx, tx, user.ID, now.Add(-displayname.ChangeWindow))
	if err != nil {
		return identity.SetDisplayNameResult{}, err
	}
	if next := displayname.NextChangeAt(inWindow, now); !next.IsZero() {
		changesTotal.Inc(ctx, outcomeLabel.Value(string(identity.DisplayNameLimitReached)))
		return identity.SetDisplayNameResult{Outcome: identity.DisplayNameLimitReached, NextChangeAt: next}, nil
	}
	if err := s.users.ChangeDisplayName(ctx, tx, user.ID, user.DisplayName, name, now); err != nil {
		return identity.SetDisplayNameResult{}, err
	}
	changesTotal.Inc(ctx, outcomeLabel.Value(outcome))
	return named, nil
}
