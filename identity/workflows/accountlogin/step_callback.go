package accountlogin

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/business/login"
	"github.com/diegobermudez03/playhoot/identity/loginproviders"
	internalrepo "github.com/diegobermudez03/playhoot/identity/workflows/accountlogin/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=accountlogin -destination=step_callback_mock_test.go . callbackRepo

// callbackRepo is HandleCallback's own narrow persistence contract.
type callbackRepo interface {
	AnswerAttempt(ctx context.Context, tx *gorm.DB, provider string, state string, answeredAt time.Time, startedAfter time.Time) (*internalrepo.AnsweredAttempt, error)
	RecordAttemptEvent(ctx context.Context, tx *gorm.DB, attemptID int64, event internalrepo.AttemptEvent) error
	FindBoundUser(ctx context.Context, tx *gorm.DB, issuer string, subject string) (int64, error)
	CreateAccount(ctx context.Context, tx *gorm.DB, userUUID string, identity internalrepo.ExternalIdentity, createdAt time.Time) (int64, bool, error)
	LinkAccount(ctx context.Context, tx *gorm.DB, guestUserID int64, identity internalrepo.ExternalIdentity, linkedAt time.Time) (bool, bool, error)
	IssueCode(ctx context.Context, tx *gorm.DB, attemptID int64, code string, expiresAt time.Time, issued internalrepo.AttemptEvent) error
}

var (
	callbackOutcomeLabel = metrics.NewLabel("outcome", "code_issued", "cancelled", "failed", "unavailable")
	callbackAccountLabel = metrics.NewLabel("account", "created", "linked_guest", "existing", "none")

	callbacksTotal = metrics.NewCounter(
		"identity_login_callbacks_total",
		"Provider answers to account sign-ins, by outcome and whether the account was created.",
		callbackOutcomeLabel, callbackAccountLabel,
	)
)

// errBoundMeanwhile rolls back an account whose external identity another
// sign-in bound first.
var errBoundMeanwhile = errors.New("external identity bound by a concurrent sign-in")

// HandleCallback takes the provider's answer to the attempt with state:
// either its code, or providerError when the person did not sign in. It
// resolves the account the proven identity belongs to, creating it on the
// first sign-in, and issues the one-time code the client completes with.
func (m *Manager) HandleCallback(ctx context.Context, provider string, state string, code string, providerError string) (identity.LoginCallbackResult, error) {
	defer logging.Step(ctx, "accountlogin.handle_callback").Close()

	now := m.now().UTC().Truncate(time.Microsecond)
	result := func(outcome identity.LoginCallbackOutcome, account string) identity.LoginCallbackResult {
		callbacksTotal.Inc(ctx, callbackOutcomeLabel.Value(string(outcome)), callbackAccountLabel.Value(account))
		return identity.LoginCallbackResult{Outcome: outcome}
	}

	chosen, ok := m.providers.Get(identity.Provider(provider))
	if !ok || state == "" {
		return result(identity.LoginFailed, "none"), nil
	}
	// Answered once, before asking the provider: a repeated callback never
	// exchanges a provider code twice.
	attempt, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (*internalrepo.AnsweredAttempt, error) {
		return m.callbackRepo.AnswerAttempt(ctx, tx, provider, state, now, now.Add(-login.AttemptLifetime))
	})
	if err != nil {
		return identity.LoginCallbackResult{}, err
	}
	if attempt == nil {
		return result(identity.LoginFailed, "none"), nil
	}
	ended := func(outcome identity.LoginCallbackOutcome, event internalrepo.Event, reason string) (identity.LoginCallbackResult, error) {
		if err := m.recordEvent(ctx, attempt.ID, internalrepo.AttemptEvent{Event: event, Reason: reason, OccurredAt: now}); err != nil {
			return identity.LoginCallbackResult{}, err
		}
		return result(outcome, "none"), nil
	}
	if providerError != "" {
		if providerError == "access_denied" {
			return ended(identity.LoginCancelled, internalrepo.EventCancelled, "")
		}
		logging.LogFields(ctx, logging.Field("provider_error", providerError))
		return ended(identity.LoginFailed, internalrepo.EventFailed, "provider_error")
	}

	external, err := chosen.Exchange(ctx, code, attempt.Nonce)
	if errors.Is(err, loginproviders.ErrInvalidAnswer) {
		return ended(identity.LoginFailed, internalrepo.EventFailed, "invalid_answer")
	}
	if err != nil {
		logging.LogError(ctx, err)
		return ended(identity.LoginUnavailable, internalrepo.EventUnavailable, "provider_unreachable")
	}

	loginCode, err := login.NewCode()
	if err != nil {
		return identity.LoginCallbackResult{}, err
	}
	bound := internalrepo.ExternalIdentity{Issuer: external.Issuer, Subject: external.Subject, Email: external.Email, Name: external.Name}
	account, err := m.issueCode(ctx, *attempt, bound, loginCode, now)
	if err != nil {
		return identity.LoginCallbackResult{}, err
	}
	issued := result(identity.LoginCodeIssued, string(account))
	issued.LoginCode = loginCode
	return issued, nil
}

// accountResolution is how a login's external identity found its User.
type accountResolution string

const (
	// resolvedExisting: the identity was already bound to an account.
	resolvedExisting accountResolution = "existing"
	// resolvedLinkedGuest: the guest who started the login became the account.
	resolvedLinkedGuest accountResolution = "linked_guest"
	// resolvedCreated: a new User was created for the account.
	resolvedCreated accountResolution = "created"
)

// issueCode resolves the external identity to its User and gives the attempt
// its code, all together. An identity bound to nobody becomes an account: of
// the guest who started the login, keeping their User, or of a new User. A
// concurrent first sign-in that binds the identity first wins; this one rolls
// back and then finds that User.
func (m *Manager) issueCode(ctx context.Context, attempt internalrepo.AnsweredAttempt, external internalrepo.ExternalIdentity, code string, now time.Time) (accountResolution, error) {
	for {
		resolution, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (accountResolution, error) {
			return m.issueCodeInTx(ctx, tx, attempt, external, code, now)
		})
		if errors.Is(err, errBoundMeanwhile) {
			continue
		}
		return resolution, err
	}
}

func (m *Manager) issueCodeInTx(ctx context.Context, tx *gorm.DB, attempt internalrepo.AnsweredAttempt, external internalrepo.ExternalIdentity, code string, now time.Time) (accountResolution, error) {
	userID, resolution, err := m.resolveAccount(ctx, tx, attempt, external, now)
	if err != nil {
		return "", err
	}
	issued := internalrepo.AttemptEvent{
		Reason: string(resolution), UserID: userID,
		ObservedEmail: external.Email, ObservedName: external.Name, OccurredAt: now,
	}
	return resolution, m.callbackRepo.IssueCode(ctx, tx, attempt.ID, code, now.Add(login.CodeLifetime), issued)
}

func (m *Manager) resolveAccount(ctx context.Context, tx *gorm.DB, attempt internalrepo.AnsweredAttempt, external internalrepo.ExternalIdentity, now time.Time) (int64, accountResolution, error) {
	userID, err := m.callbackRepo.FindBoundUser(ctx, tx, external.Issuer, external.Subject)
	if err != nil || userID != 0 {
		return userID, resolvedExisting, err
	}

	// if no user existed, this is a new account registration, not login
	if attempt.GuestUserID != nil {
		linked, bound, err := m.callbackRepo.LinkAccount(ctx, tx, *attempt.GuestUserID, external, now)
		switch {
		case err != nil:
			return 0, "", err
		case linked && !bound:
			return 0, "", errBoundMeanwhile
		case linked:
			return *attempt.GuestUserID, resolvedLinkedGuest, nil
		}
		// That guest is an account already (another login linked it first):
		// a new User, as without a guest.
	}
	userID, bound, err := m.callbackRepo.CreateAccount(ctx, tx, uuid.NewString(), external, now)
	if err != nil {
		return 0, "", err
	}
	if !bound {
		return 0, "", errBoundMeanwhile
	}
	return userID, resolvedCreated, nil
}

func (m *Manager) recordEvent(ctx context.Context, attemptID int64, event internalrepo.AttemptEvent) error {
	_, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (struct{}, error) {
		return struct{}{}, m.callbackRepo.RecordAttemptEvent(ctx, tx, attemptID, event)
	})
	return err
}
