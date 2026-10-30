package accountlogin

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/business/login"
	"github.com/diegobermudez03/playhoot/identity/internal/business/refreshtoken"
	internalrepo "github.com/diegobermudez03/playhoot/identity/workflows/accountlogin/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=accountlogin -destination=step_complete_mock_test.go . completeRepo,accessIssuer

// completeRepo is CompleteLogin's own narrow persistence contract.
type completeRepo interface {
	LockAttemptByCode(ctx context.Context, tx *gorm.DB, code string) (*internalrepo.CodeAttempt, error)
	CloseAttempt(ctx context.Context, tx *gorm.DB, attemptID int64, closed internalrepo.AttemptEvent) error
	OpenSession(ctx context.Context, tx *gorm.DB, session internalrepo.NewSession) error
	EndSessionOf(ctx context.Context, tx *gorm.DB, refreshToken string, endedAt time.Time) error
}

var (
	completeOutcomeLabel = metrics.NewLabel("outcome", "completed", "login_code_invalid")

	completionsTotal = metrics.NewCounter(
		"identity_login_completions_total",
		"Account sign-in completions, by outcome.",
		completeOutcomeLabel,
	)
)

// signedIn is an account session just opened.
type signedIn struct {
	user         identity.User
	refreshToken string
}

// CompleteLogin exchanges a one-time code and the verifier of the login it
// started for the account's credentials. replacesRefreshToken, when given, is
// the session this client held until now (a guest's): it ends with the login.
func (m *Manager) CompleteLogin(ctx context.Context, code string, codeVerifier string, replacesRefreshToken string) (identity.CompleteLoginResult, error) {
	defer logging.Step(ctx, "accountlogin.complete_login").Close()

	now := m.now().UTC().Truncate(time.Microsecond)
	opened, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (*signedIn, error) {
		return m.completeInTx(ctx, tx, code, codeVerifier, replacesRefreshToken, now)
	})
	if err != nil {
		return identity.CompleteLoginResult{}, err
	}
	if opened == nil {
		completionsTotal.Inc(ctx, completeOutcomeLabel.Value("login_code_invalid"))
		return identity.CompleteLoginResult{Outcome: identity.LoginCodeInvalid}, nil
	}
	accessToken, expiresAt, err := m.access.Issue(opened.user, now)
	if err != nil {
		return identity.CompleteLoginResult{}, err
	}
	completionsTotal.Inc(ctx, completeOutcomeLabel.Value("completed"))
	return identity.CompleteLoginResult{
		Outcome: identity.LoginCompleted,
		Credentials: identity.Credentials{
			User:            opened.user,
			AccessToken:     accessToken,
			AccessIssuedAt:  now,
			AccessExpiresAt: expiresAt,
			RefreshToken:    opened.refreshToken,
		},
	}, nil
}

// completeInTx uses up the code and opens the session; nil when the code
// cannot complete a login. Every refusal of a known code is kept in the
// attempt's history, and a wrong verifier uses the code up, so it cannot be
// guessed at.
func (m *Manager) completeInTx(ctx context.Context, tx *gorm.DB, code string, codeVerifier string, replacesRefreshToken string, now time.Time) (*signedIn, error) {
	if code == "" {
		return nil, nil
	}
	attempt, err := m.completeRepo.LockAttemptByCode(ctx, tx, code)
	if err != nil || attempt == nil {
		return nil, err
	}
	rejected := func(reason string) (*signedIn, error) {
		return nil, m.completeRepo.CloseAttempt(ctx, tx, attempt.ID, internalrepo.AttemptEvent{Event: internalrepo.EventRejected, Reason: reason, OccurredAt: now})
	}
	switch {
	case attempt.CompletedAt != nil:
		return rejected("used")
	case !now.Before(attempt.ExpiresAt):
		return rejected("expired")
	case !login.VerifierMatches(attempt.CodeChallenge, codeVerifier):
		return rejected("verifier_mismatch")
	}

	refreshToken, err := m.newRefresh()
	if err != nil {
		return nil, err
	}
	err = m.completeRepo.OpenSession(ctx, tx, internalrepo.NewSession{
		SessionUUID:  uuid.NewString(),
		UserID:       attempt.UserID,
		RefreshToken: refreshToken,
		IssuedAt:     now,
		ExpiresAt:    refreshtoken.ExpiresAt(identity.LevelAccount, now, now),
	})
	if err != nil {
		return nil, err
	}
	if replacesRefreshToken != "" {
		if err := m.completeRepo.EndSessionOf(ctx, tx, replacesRefreshToken, now); err != nil {
			return nil, err
		}
	}
	err = m.completeRepo.CloseAttempt(ctx, tx, attempt.ID, internalrepo.AttemptEvent{Event: internalrepo.EventCompleted, UserID: attempt.UserID, OccurredAt: now})
	if err != nil {
		return nil, err
	}
	user := identity.User{UUID: attempt.UserUUID, DisplayName: attempt.DisplayName, Level: identity.LevelAccount}
	return &signedIn{user: user, refreshToken: refreshToken}, nil
}
