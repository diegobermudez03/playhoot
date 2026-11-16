package emaillogin

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/business/emailcode"
	internalrepo "github.com/diegobermudez03/playhoot/identity/workflows/emaillogin/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=emaillogin -destination=step_link_mock_test.go . linkRepo

// linkRepo is the email-link steps' own narrow persistence contract.
type linkRepo interface {
	LockLiveAttempt(ctx context.Context, tx *gorm.DB, email string, now time.Time) (*internalrepo.LiveAttempt, error)
	RecordEvent(ctx context.Context, tx *gorm.DB, attemptID int64, event internalrepo.Event, at time.Time) error
	CloseAttempt(ctx context.Context, tx *gorm.DB, attemptID int64, event internalrepo.Event, at time.Time) error
	FindBoundUser(ctx context.Context, tx *gorm.DB, issuer string, subject string) (*internalrepo.BoundIdentity, error)
	BindIdentity(ctx context.Context, tx *gorm.DB, userID int64, bound internalrepo.ExternalIdentity, boundAt time.Time) (bool, error)
	UserIDOf(ctx context.Context, tx *gorm.DB, userUUID string) (int64, error)
}

var (
	linkOutcomeLabel = metrics.NewLabel("outcome", string(identity.MethodLinked), string(identity.MethodInUse),
		string(identity.MethodInvalidCode), string(identity.MethodAttemptGone))

	emailLinksTotal = metrics.NewCounter(
		"identity_email_method_links_total",
		"Attempts to add an address as a way into an account, by what was decided.",
		linkOutcomeLabel,
	)
)

// StartLinkEmail sends a one-time code to the address userUUID wants to add
// as another way into their account. It answers like a sign-in start: the
// acknowledgement says nothing about the address.
func (m *Manager) StartLinkEmail(ctx context.Context, userUUID string, address string, language string) (identity.StartEmailLoginResult, error) {
	defer logging.Step(ctx, "emaillogin.start_link_email").Close()
	return m.start(ctx, address, language, internalrepo.IntentLink, "", userUUID)
}

// CompleteLinkEmail verifies the code sent to address and adds it to
// userUUID's account. An address already on another account is refused:
// accounts are never merged.
func (m *Manager) CompleteLinkEmail(ctx context.Context, userUUID string, address string, code string) (identity.LinkMethodResult, error) {
	defer logging.Step(ctx, "emaillogin.complete_link_email").Close()

	normalized, ok := emailcode.Normalize(address)
	if !ok {
		return m.linkDecided(ctx, identity.LinkMethodResult{Outcome: identity.MethodAttemptGone}), nil
	}
	now := m.now().UTC().Truncate(time.Microsecond)
	for {
		result, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (identity.LinkMethodResult, error) {
			return m.linkEmailInTx(ctx, tx, userUUID, normalized, code, now)
		})
		if errors.Is(err, errBoundMeanwhile) {
			continue
		}
		if err != nil {
			return identity.LinkMethodResult{}, err
		}
		return m.linkDecided(ctx, result), nil
	}
}

// linkDecided counts a link decision and answers it.
func (m *Manager) linkDecided(ctx context.Context, result identity.LinkMethodResult) identity.LinkMethodResult {
	emailLinksTotal.Inc(ctx, linkOutcomeLabel.Value(string(result.Outcome)))
	return result
}

func (m *Manager) linkEmailInTx(ctx context.Context, tx *gorm.DB, userUUID string, address string, code string, now time.Time) (identity.LinkMethodResult, error) {
	gone := identity.LinkMethodResult{Outcome: identity.MethodAttemptGone}
	userID, err := m.linkRepo.UserIDOf(ctx, tx, userUUID)
	if err != nil || userID == 0 {
		return gone, err
	}
	attempt, err := m.linkRepo.LockLiveAttempt(ctx, tx, address, now)
	if err != nil || attempt == nil {
		return gone, err
	}
	if internalrepo.Intent(attempt.Intent) != internalrepo.IntentLink || attempt.LinkUserID == nil || *attempt.LinkUserID != userID {
		return gone, nil
	}
	if !emailcode.Matches(attempt.CodeSha256, code) {
		outcome, err := m.recordWrongCode(ctx, tx, attempt, now)
		if err != nil || outcome == identity.EmailLoginAttemptGone {
			return gone, err
		}
		return identity.LinkMethodResult{Outcome: identity.MethodInvalidCode}, nil
	}

	issuer := string(identity.ProviderEmail)
	existing, err := m.linkRepo.FindBoundUser(ctx, tx, issuer, address)
	if err != nil {
		return identity.LinkMethodResult{}, err
	}
	if existing != nil && existing.UserID != userID {
		if err := m.linkRepo.CloseAttempt(ctx, tx, attempt.ID, internalrepo.EventRefusedInUse, now); err != nil {
			return identity.LinkMethodResult{}, err
		}
		return identity.LinkMethodResult{Outcome: identity.MethodInUse}, nil
	}
	if existing == nil {
		bound, err := m.linkRepo.BindIdentity(ctx, tx, userID, internalrepo.ExternalIdentity{Issuer: issuer, Subject: address, Email: address}, now)
		if err != nil {
			return identity.LinkMethodResult{}, err
		}
		if !bound {
			return identity.LinkMethodResult{}, errBoundMeanwhile
		}
	}
	if err := m.linkRepo.CloseAttempt(ctx, tx, attempt.ID, internalrepo.EventCompleted, now); err != nil {
		return identity.LinkMethodResult{}, err
	}
	return identity.LinkMethodResult{Outcome: identity.MethodLinked}, nil
}
