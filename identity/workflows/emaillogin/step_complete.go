package emaillogin

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/business/emailcode"
	"github.com/diegobermudez03/playhoot/identity/internal/business/refreshtoken"
	internalrepo "github.com/diegobermudez03/playhoot/identity/workflows/emaillogin/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=emaillogin -destination=step_complete_mock_test.go . completeRepo

// completeRepo is CompleteEmailLogin's own narrow persistence contract.
type completeRepo interface {
	LockLiveAttempt(ctx context.Context, tx *gorm.DB, email string, now time.Time) (*internalrepo.LiveAttempt, error)
	RecordEvent(ctx context.Context, tx *gorm.DB, attemptID int64, event internalrepo.Event, at time.Time) error
	CloseAttempt(ctx context.Context, tx *gorm.DB, attemptID int64, event internalrepo.Event, at time.Time) error
	FindBoundUser(ctx context.Context, tx *gorm.DB, issuer string, subject string) (*internalrepo.BoundIdentity, error)
	ReadUser(ctx context.Context, tx *gorm.DB, userID int64) (internalrepo.Identified, error)
	CreateAccount(ctx context.Context, tx *gorm.DB, userUUID string, bound internalrepo.ExternalIdentity, createdAt time.Time) (int64, bool, error)
	LinkAccount(ctx context.Context, tx *gorm.DB, guestUserID int64, bound internalrepo.ExternalIdentity, linkedAt time.Time) (bool, bool, error)
	OpenSession(ctx context.Context, tx *gorm.DB, opening internalrepo.NewSession) error
	EndSessionOf(ctx context.Context, tx *gorm.DB, refreshToken string, endedAt time.Time) error
}

var (
	completeOutcomeLabel = metrics.NewLabel("outcome", "completed", "invalid_code", "attempt_gone")
	completeAccountLabel = metrics.NewLabel("account", "created", "linked_guest", "existing", "none")

	completionsTotal = metrics.NewCounter(
		"identity_email_code_completions_total",
		"One-time email codes submitted, by what was decided and whether an account was created.",
		completeOutcomeLabel, completeAccountLabel,
	)
)

// errBoundMeanwhile rolls back an account whose address another sign-in
// bound first.
var errBoundMeanwhile = errors.New("address bound by a concurrent sign-in")

// accountResolution is how a completed code found its User.
type accountResolution string

const (
	// resolvedExisting: the address was already a way into an account.
	resolvedExisting accountResolution = "existing"
	// resolvedLinkedGuest: the guest who asked for the code became the account.
	resolvedLinkedGuest accountResolution = "linked_guest"
	// resolvedCreated: a new User was created for the account.
	resolvedCreated accountResolution = "created"
)

// signedIn is an account session just opened.
type signedIn struct {
	user         identity.User
	refreshToken string
	resolution   accountResolution
}

// CompleteEmailLogin exchanges an address and the code sent to it for that
// account's credentials, registering the account when the address was not a
// way into one yet. replacesRefreshToken, when given, is the session this
// client held until now (a guest's): it ends with the sign-in.
func (m *Manager) CompleteEmailLogin(ctx context.Context, address string, code string, replacesRefreshToken string) (identity.CompleteEmailLoginResult, error) {
	defer logging.Step(ctx, "emaillogin.complete_email_login").Close()

	normalized, ok := emailcode.Normalize(address)
	if !ok {
		return m.refused(ctx, identity.EmailLoginAttemptGone), nil
	}
	now := m.now().UTC().Truncate(time.Microsecond)
	var opened *signedIn
	var outcome identity.CompleteEmailLoginOutcome
	for {
		var err error
		opened, outcome, err = m.completeOnce(ctx, normalized, code, replacesRefreshToken, now)
		if errors.Is(err, errBoundMeanwhile) {
			continue
		}
		if err != nil {
			return identity.CompleteEmailLoginResult{}, err
		}
		break
	}
	if opened == nil {
		return m.refused(ctx, outcome), nil
	}
	accessToken, expiresAt, err := m.access.Issue(opened.user, now)
	if err != nil {
		return identity.CompleteEmailLoginResult{}, err
	}
	completionsTotal.Inc(ctx, completeOutcomeLabel.Value("completed"), completeAccountLabel.Value(string(opened.resolution)))
	return identity.CompleteEmailLoginResult{
		Outcome: identity.EmailLoginCompleted,
		Credentials: identity.Credentials{
			User:            opened.user,
			AccessToken:     accessToken,
			AccessIssuedAt:  now,
			AccessExpiresAt: expiresAt,
			RefreshToken:    opened.refreshToken,
		},
	}, nil
}

// refused counts a completion that opened nothing and answers why.
func (m *Manager) refused(ctx context.Context, outcome identity.CompleteEmailLoginOutcome) identity.CompleteEmailLoginResult {
	completionsTotal.Inc(ctx, completeOutcomeLabel.Value(string(outcome)), completeAccountLabel.Value("none"))
	return identity.CompleteEmailLoginResult{Outcome: outcome}
}

// attempted is one run of the completion transaction.
type attempted struct {
	opened  *signedIn
	outcome identity.CompleteEmailLoginOutcome
}

func (m *Manager) completeOnce(ctx context.Context, address string, code string, replacesRefreshToken string, now time.Time) (*signedIn, identity.CompleteEmailLoginOutcome, error) {
	result, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (attempted, error) {
		opened, outcome, err := m.completeInTx(ctx, tx, address, code, replacesRefreshToken, now)
		return attempted{opened: opened, outcome: outcome}, err
	})
	return result.opened, result.outcome, err
}

// completeInTx uses the code up and opens the session; a nil session comes
// with the outcome that refused it.
func (m *Manager) completeInTx(ctx context.Context, tx *gorm.DB, address string, code string, replacesRefreshToken string, now time.Time) (*signedIn, identity.CompleteEmailLoginOutcome, error) {
	attempt, err := m.completeRepo.LockLiveAttempt(ctx, tx, address, now)
	if err != nil || attempt == nil {
		return nil, identity.EmailLoginAttemptGone, err
	}
	if internalrepo.Intent(attempt.Intent) != internalrepo.IntentSignIn {
		return nil, identity.EmailLoginAttemptGone, nil
	}
	if !emailcode.Matches(attempt.CodeSha256, code) {
		outcome, err := m.recordWrongCode(ctx, tx, attempt, now)
		return nil, outcome, err
	}

	resolved, resolution, err := m.resolve(ctx, tx, attempt, address, now)
	if err != nil {
		return nil, "", err
	}
	refreshToken, err := m.newRefresh()
	if err != nil {
		return nil, "", err
	}
	err = m.completeRepo.OpenSession(ctx, tx, internalrepo.NewSession{
		SessionUUID:  uuid.NewString(),
		UserID:       resolved.UserID,
		RefreshToken: refreshToken,
		IssuedAt:     now,
		ExpiresAt:    refreshtoken.ExpiresAt(identity.LevelAccount, now, now),
	})
	if err != nil {
		return nil, "", err
	}
	if replacesRefreshToken != "" {
		if err := m.completeRepo.EndSessionOf(ctx, tx, replacesRefreshToken, now); err != nil {
			return nil, "", err
		}
	}
	if err := m.completeRepo.CloseAttempt(ctx, tx, attempt.ID, internalrepo.EventCompleted, now); err != nil {
		return nil, "", err
	}
	user := identity.User{
		UUID: resolved.UserUUID, DisplayName: resolved.DisplayName, Level: identity.LevelAccount,
		Mode: identity.NormalizeMode(identity.Mode(resolved.Mode)),
	}
	return &signedIn{user: user, refreshToken: refreshToken, resolution: resolution}, identity.EmailLoginCompleted, nil
}

// recordWrongCode keeps the wrong code in the attempt's history and says
// whether the attempt survives it.
func (m *Manager) recordWrongCode(ctx context.Context, tx *gorm.DB, attempt *internalrepo.LiveAttempt, now time.Time) (identity.CompleteEmailLoginOutcome, error) {
	if err := m.completeRepo.RecordEvent(ctx, tx, attempt.ID, internalrepo.EventWrongCode, now); err != nil {
		return "", err
	}
	if attempt.WrongCodes+1 < emailcode.MaxWrongCodes {
		return identity.EmailLoginInvalidCode, nil
	}
	// Exhausted: the attempt answers no more codes, and asking for a new one
	// is how a person carries on.
	if err := m.completeRepo.RecordEvent(ctx, tx, attempt.ID, internalrepo.EventExhausted, now); err != nil {
		return "", err
	}
	return identity.EmailLoginAttemptGone, nil
}

// resolve finds the User the proven address belongs to, exactly as a
// provider callback does: an address bound to nobody becomes an account, of
// the guest who asked for the code or of a new User. A concurrent sign-in
// that binds it first wins; this one rolls back and finds that User.
func (m *Manager) resolve(ctx context.Context, tx *gorm.DB, attempt *internalrepo.LiveAttempt, address string, now time.Time) (internalrepo.Identified, accountResolution, error) {
	existing, err := m.completeRepo.FindBoundUser(ctx, tx, string(identity.ProviderEmail), address)
	if err != nil {
		return internalrepo.Identified{}, "", err
	}
	if existing != nil {
		resolved, err := m.completeRepo.ReadUser(ctx, tx, existing.UserID)
		return resolved, resolvedExisting, err
	}
	binding := internalrepo.ExternalIdentity{Issuer: string(identity.ProviderEmail), Subject: address, Email: address}
	if attempt.GuestUserID != nil {
		linked, bound, err := m.completeRepo.LinkAccount(ctx, tx, *attempt.GuestUserID, binding, now)
		switch {
		case err != nil:
			return internalrepo.Identified{}, "", err
		case linked && !bound:
			return internalrepo.Identified{}, "", errBoundMeanwhile
		case linked:
			resolved, err := m.completeRepo.ReadUser(ctx, tx, *attempt.GuestUserID)
			return resolved, resolvedLinkedGuest, err
		}
		// That guest is an account already (another sign-in linked it
		// first): a new User, as without a guest.
	}
	userID, bound, err := m.completeRepo.CreateAccount(ctx, tx, uuid.NewString(), binding, now)
	if err != nil {
		return internalrepo.Identified{}, "", err
	}
	if !bound {
		return internalrepo.Identified{}, "", errBoundMeanwhile
	}
	resolved, err := m.completeRepo.ReadUser(ctx, tx, userID)
	return resolved, resolvedCreated, err
}
