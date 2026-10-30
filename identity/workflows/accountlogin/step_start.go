package accountlogin

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/business/login"
	internalrepo "github.com/diegobermudez03/playhoot/identity/workflows/accountlogin/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=accountlogin -destination=step_start_mock_test.go . startRepo,providers

// startRepo is StartLogin's own narrow persistence contract.
type startRepo interface {
	CreateAttempt(ctx context.Context, tx *gorm.DB, attempt internalrepo.NewAttempt) error
}

var (
	startOutcomeLabel = metrics.NewLabel("outcome", "started", "provider_unavailable", "invalid_code_challenge")

	startsTotal = metrics.NewCounter(
		"identity_login_starts_total",
		"Account sign-ins started, by outcome.",
		startOutcomeLabel,
	)
)

// StartLogin starts a sign-in with provider for a client that keeps the
// verifier codeChallenge (S256) was made from, and answers where the person
// proves their identity. guestUserUUID, when set, is the guest the client is:
// if the login creates an account, it is that guest's.
func (m *Manager) StartLogin(ctx context.Context, provider string, codeChallenge string, guestUserUUID string) (identity.StartLoginResult, error) {
	defer logging.Step(ctx, "accountlogin.start_login").Close()

	chosen, ok := m.providers.Get(identity.Provider(provider))
	if !ok {
		startsTotal.Inc(ctx, startOutcomeLabel.Value("provider_unavailable"))
		return identity.StartLoginResult{Outcome: identity.LoginProviderUnavailable}, nil
	}
	if !login.ValidChallenge(codeChallenge) {
		startsTotal.Inc(ctx, startOutcomeLabel.Value("invalid_code_challenge"))
		return identity.StartLoginResult{Outcome: identity.LoginInvalidCodeChallenge}, nil
	}
	state, err := login.NewState()
	if err != nil {
		return identity.StartLoginResult{}, err
	}
	nonce, err := login.NewNonce()
	if err != nil {
		return identity.StartLoginResult{}, err
	}
	attempt := internalrepo.NewAttempt{
		State:         state,
		Provider:      provider,
		CodeChallenge: codeChallenge,
		Nonce:         nonce,
		GuestUserUUID: guestUserUUID,
		StartedAt:     m.now().UTC().Truncate(time.Microsecond),
	}
	_, err = utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (struct{}, error) {
		return struct{}{}, m.startRepo.CreateAttempt(ctx, tx, attempt)
	})
	if err != nil {
		return identity.StartLoginResult{}, err
	}
	startsTotal.Inc(ctx, startOutcomeLabel.Value("started"))
	return identity.StartLoginResult{Outcome: identity.LoginStarted, AuthorizationURL: chosen.AuthorizationURL(state, nonce)}, nil
}
