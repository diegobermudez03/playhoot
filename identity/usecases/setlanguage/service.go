// Package setlanguage stores the language an account reads Playhoot in, so
// the interface starts in it on a new browser and emails are written in it.
// It never says anything about where the person lives.
package setlanguage

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity"
	internalrepo "github.com/diegobermudez03/playhoot/identity/usecases/setlanguage/internal/repo"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=setlanguage -destination=service_mock_test.go . userStore

// userStore reads and changes a User's interface language inside a
// transaction.
type userStore interface {
	LockUser(ctx context.Context, tx *gorm.DB, userUUID string) (*internalrepo.User, error)
	ChangeLanguage(ctx context.Context, tx *gorm.DB, userID int64, fromLanguage string, language string, changedAt time.Time) error
}

var (
	outcomeLabel = metrics.NewLabel("outcome", "chosen",
		string(identity.LanguageInvalid), string(identity.LanguageNotAccount))

	choicesTotal = metrics.NewCounter(
		"identity_interface_language_choices_total",
		"Interface languages chosen, by whether the choice was stored or refused.",
		outcomeLabel,
	)
)

// Service is Identity's interface language use case.
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

// SetLanguage stores language as the interface language of userUUID.
// identity.ErrUserNotFound when there is no such User.
func (s *Service) SetLanguage(ctx context.Context, userUUID string, language string) (identity.SetLanguageOutcome, error) {
	if !identity.ValidLanguage(language) {
		choicesTotal.Inc(ctx, outcomeLabel.Value(string(identity.LanguageInvalid)))
		return identity.LanguageInvalid, nil
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	outcome, err := utils.RunInDBTransaction(ctx, s.db, func(ctx context.Context, tx *gorm.DB) (identity.SetLanguageOutcome, error) {
		return s.setInTx(ctx, tx, userUUID, language, now)
	})
	if err != nil {
		return "", err
	}
	metric := string(outcome)
	if outcome == identity.LanguageSet {
		metric = "chosen"
	}
	choicesTotal.Inc(ctx, outcomeLabel.Value(metric))
	return outcome, nil
}

func (s *Service) setInTx(ctx context.Context, tx *gorm.DB, userUUID string, language string, now time.Time) (identity.SetLanguageOutcome, error) {
	user, err := s.users.LockUser(ctx, tx, userUUID)
	if err != nil {
		return "", err
	}
	if user == nil {
		return "", identity.ErrUserNotFound
	}
	if !user.HasAccount {
		return identity.LanguageNotAccount, nil
	}
	if user.Language == language {
		return identity.LanguageSet, nil
	}
	if err := s.users.ChangeLanguage(ctx, tx, user.ID, user.Language, language, now); err != nil {
		return "", err
	}
	return identity.LanguageSet, nil
}
